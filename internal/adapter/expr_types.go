package adapter

import (
	"context"
	"fmt"
	"go/ast"
	"go/build"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"gooze.dev/pkg/gooze/internal/domain"
)

// ExprTypes type-checks a package's source with go/types against the export
// data of its imports, which go list -export reads from the build cache (or
// builds). One importer serves every package, so an import is loaded once per
// run, and each package's result is kept by its directory.
type ExprTypes struct {
	mu       sync.Mutex
	fset     *token.FileSet
	importer types.Importer
	exports  map[string]string               // import path -> export data file
	env      []string                        // of go list, which builds the export data
	packages map[string]map[string]fileExprs // dir -> file -> its expressions' kinds
}

// fileExprs are the spans of a file's expressions by kind.
type fileExprs struct {
	strings, indexed map[domain.Span]bool
}

// NewExprTypes returns an ExprTypes with nothing checked yet.
func NewExprTypes() *ExprTypes {
	e := &ExprTypes{
		fset:     token.NewFileSet(),
		exports:  map[string]string{},
		packages: map[string]map[string]fileExprs{},
	}
	e.importer = importer.ForCompiler(e.fset, "gc", e.lookup)

	return e
}

// lookup opens the export data of the package at path.
func (e *ExprTypes) lookup(path string) (io.ReadCloser, error) {
	file, ok := e.exports[path]
	if !ok {
		return nil, fmt.Errorf("no export data for %q", path)
	}

	return os.Open(file) //nolint:gosec // a file go list named.
}

// SetBuildEnv adds env to the environment of the go list that builds export
// data, so it builds in the same cache as the mutants (an isolated one leaves
// the shared GOCACHE alone).
func (e *ExprTypes) SetBuildEnv(env []string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.env = env
}

// loadExports records the export data files of the dependencies of the
// package in dir. Packages that don't build have none, so their types stay
// unknown.
func (e *ExprTypes) loadExports(ctx context.Context, dir string) {
	cmd := exec.CommandContext(ctx, "go", "list", "-e", "-export", "-deps", "-f", "{{if .Export}}{{.ImportPath}}={{.Export}}{{end}}", ".")
	cmd.Dir = dir

	cmd.Env = append(os.Environ(), e.env...)

	out, err := cmd.Output()
	if err != nil {
		return
	}

	for line := range strings.Lines(string(out)) {
		if path, file, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
			e.exports[path] = file
		}
	}
}

// StringExprs implements domain.ExprTypes.
func (e *ExprTypes) StringExprs(ctx context.Context, file string) map[domain.Span]bool {
	return e.file(ctx, file).strings
}

// IndexedExprs implements domain.ExprTypes.
func (e *ExprTypes) IndexedExprs(ctx context.Context, file string) map[domain.Span]bool {
	return e.file(ctx, file).indexed
}

// file returns the expressions of file, type-checking its package the first
// time one of its files is asked for.
func (e *ExprTypes) file(ctx context.Context, file string) fileExprs {
	file = filepath.Clean(file)
	dir := filepath.Dir(file)

	e.mu.Lock()
	defer e.mu.Unlock()

	pkg, ok := e.packages[dir]
	if !ok {
		pkg = e.check(ctx, dir)
		e.packages[dir] = pkg
	}

	return pkg[file]
}

// check type-checks the package in dir without its tests and returns the
// kinds of the expressions of each of its files. Type errors leave the
// expressions they touch untyped; the rest is still checked.
func (e *ExprTypes) check(ctx context.Context, dir string) map[string]fileExprs {
	files := e.parse(dir)
	// Equivalent: a check of no files finds no expression either; returning
	// early only saves the go list.
	if len(files) == 0 { //gooze:ignore comparison
		return nil
	}

	e.loadExports(ctx, dir)

	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}}
	conf := types.Config{Importer: e.importer, Error: func(error) {}}
	// Errors leave the expressions they touch untyped.
	_, _ = conf.Check(dir, e.fset, files, info)

	exprs := map[string]fileExprs{}

	for expr, tv := range info.Types {
		start, end := e.fset.Position(expr.Pos()), e.fset.Position(expr.End())
		span := domain.Span{Start: start.Offset, End: end.Offset}

		f, ok := exprs[start.Filename]
		if !ok {
			f = fileExprs{strings: map[domain.Span]bool{}, indexed: map[domain.Span]bool{}}
			exprs[start.Filename] = f
		}

		if isString(tv.Type) {
			f.strings[span] = true
		}

		if isIndexed(tv.Type) {
			f.indexed[span] = true
		}
	}

	return exprs
}

// parse parses the non-test files of dir that the build context selects.
func (e *ExprTypes) parse(dir string) []*ast.File {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var files []*ast.File

	for _, entry := range entries {
		if !packageSource(dir, entry) {
			continue
		}

		// A file of another package is left out by the type check itself.
		file, err := parser.ParseFile(e.fset, filepath.Join(dir, entry.Name()), nil, parser.SkipObjectResolution)
		if err != nil {
			continue
		}

		files = append(files, file)
	}

	return files
}

// packageSource reports whether entry is a non-test file of dir that the
// build context selects. A selected file that isn't Go, such as assembly,
// fails to parse and is left out there.
func packageSource(dir string, entry os.DirEntry) bool {
	if strings.HasSuffix(entry.Name(), "_test.go") {
		return false
	}

	match, err := build.Default.MatchFile(dir, entry.Name())

	return err == nil && match
}

// isString reports whether t is a string type: string, an untyped string
// constant or a defined type whose underlying type is string.
func isString(t types.Type) bool {
	basic, ok := t.Underlying().(*types.Basic)

	return ok && (basic.Kind() == types.String || basic.Kind() == types.UntypedString)
}

// isIndexed reports whether a range over a value of type t gives keys in
// [0, len): a slice, an array or a pointer to one, a string or an integer.
func isIndexed(t types.Type) bool {
	switch u := t.Underlying().(type) {
	case *types.Slice, *types.Array:
		return true
	case *types.Pointer:
		_, ok := u.Elem().Underlying().(*types.Array)

		return ok
	case *types.Basic:
		return slices.Contains(indexedKinds, u.Kind())
	default:
		return false
	}
}

// indexedKinds are the basic types a range gives keys in [0, len) over: the
// strings and the integers.
var indexedKinds = []types.BasicKind{
	types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
	types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64, types.Uintptr,
	types.String, types.UntypedInt, types.UntypedRune, types.UntypedString,
}
