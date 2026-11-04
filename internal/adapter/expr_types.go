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
	exports  map[string]string                          // import path -> export data file
	strings  map[string]map[string]map[domain.Span]bool // dir -> file -> string spans
}

// NewExprTypes returns an ExprTypes with nothing checked yet.
func NewExprTypes() *ExprTypes {
	e := &ExprTypes{
		fset:    token.NewFileSet(),
		exports: map[string]string{},
		strings: map[string]map[string]map[domain.Span]bool{},
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

// loadExports records the export data files of the dependencies of the
// package in dir. Packages that don't build have none, so their types stay
// unknown.
func (e *ExprTypes) loadExports(ctx context.Context, dir string) {
	cmd := exec.CommandContext(ctx, "go", "list", "-e", "-export", "-deps", "-f", "{{if .Export}}{{.ImportPath}}={{.Export}}{{end}}", ".")
	cmd.Dir = dir

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
	file = filepath.Clean(file)
	dir := filepath.Dir(file)

	e.mu.Lock()
	defer e.mu.Unlock()

	pkg, ok := e.strings[dir]
	if !ok {
		pkg = e.check(ctx, dir)
		e.strings[dir] = pkg
	}

	return pkg[file]
}

// check type-checks the package in dir without its tests and returns the
// string spans of each of its files. Type errors leave the expressions they
// touch untyped; the rest is still checked.
func (e *ExprTypes) check(ctx context.Context, dir string) map[string]map[domain.Span]bool {
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

	spans := map[string]map[domain.Span]bool{}

	for expr, tv := range info.Types {
		if !isString(tv.Type) {
			continue
		}

		start, end := e.fset.Position(expr.Pos()), e.fset.Position(expr.End())
		if spans[start.Filename] == nil {
			spans[start.Filename] = map[domain.Span]bool{}
		}

		spans[start.Filename][domain.Span{Start: start.Offset, End: end.Offset}] = true
	}

	return spans
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
