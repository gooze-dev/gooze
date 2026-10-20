// Package adapter contains UI and infrastructure adapters for the Gooze CLI.
package adapter

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// LocalSourceFSAdapter is the concrete implementation that will back the
// SourceFSAdapter interface. It currently returns ErrNotImplemented so tests
// can drive the actual logic.
type LocalSourceFSAdapter struct{}

// NewLocalSourceFSAdapter constructs a LocalSourceFSAdapter instance ready to
// be wired into the workflow.
func NewLocalSourceFSAdapter() *LocalSourceFSAdapter {
	return &LocalSourceFSAdapter{}
}

// Get collects Go source files for the provided roots and returns SourceV2 entries.
func (a *LocalSourceFSAdapter) Get(ctx context.Context, roots []m.Path, ignore ...string) ([]rpt.Source, error) {
	sources := make([]rpt.Source, 0, len(roots))

	err := a.eachSource(ctx, roots, ignore, func(source rpt.Source) error {
		sources = append(sources, source)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return sources, nil
}

// Stream scans the roots and emits sources on a channel as they are discovered.
func (a *LocalSourceFSAdapter) Stream(ctx context.Context, roots []m.Path, ignore ...string) (<-chan rpt.Source, <-chan error) {
	out := make(chan rpt.Source)
	errc := make(chan error, 1)

	go func() {
		defer close(out)
		defer close(errc)

		err := a.eachSource(ctx, roots, ignore, func(source rpt.Source) error {
			select {
			case out <- source:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		if err != nil {
			errc <- err
		}
	}()

	return out, errc
}

// eachSource walks the roots and invokes emit once for each unique source. It is
// the shared core behind both Get (buffering) and Stream (channeling).
func (a *LocalSourceFSAdapter) eachSource(ctx context.Context, roots []m.Path, ignore []string, emit func(rpt.Source) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if len(roots) == 0 {
		return nil
	}

	ignoreRegexps, err := compileIgnoreRegexps(ignore)
	if err != nil {
		return err
	}

	emitUnique := dedupEmit(make(map[string]struct{}), emit)

	for _, root := range roots {
		if err := a.collectSourcesFromRoot(ctx, root, ignoreRegexps, emitUnique); err != nil {
			return err
		}
	}

	return nil
}

func (a *LocalSourceFSAdapter) collectSourcesFromRoot(ctx context.Context, root m.Path, ignoreRegexps []*regexp.Regexp, emit func(rpt.Source) error) error {
	rootPath, recursive, err := normalizeRootPath(string(root))
	if err != nil {
		return err
	}

	info, err := a.FileInfo(ctx, m.Path(rootPath))
	if err != nil {
		return fmt.Errorf("root path error: %w", err)
	}

	if info.IsDir() {
		return a.collectSourcesFromDir(ctx, rootPath, recursive, ignoreRegexps, emit)
	}

	return a.emitFileSource(ctx, rootPath, ignoreRegexps, emit)
}

// emitFileSource emits the source for a single file path, skipping files that
// are not valid mutation sources.
func (a *LocalSourceFSAdapter) emitFileSource(ctx context.Context, path string, ignoreRegexps []*regexp.Regexp, emit func(rpt.Source) error) error {
	source, err := a.processFilePath(ctx, path, ignoreRegexps)
	if err != nil {
		if isInvalidSourceErr(err) {
			slog.Warn("Skipping source file that could not be parsed", "path", path, "error", err)
			return nil
		}

		return err
	}

	return emit(source)
}

// Walk iterates over files under root, optionally descending into subdirectories.
func (a *LocalSourceFSAdapter) Walk(ctx context.Context, root m.Path, recursive bool, fn domain.FilepathWalkFunc) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	rootStr := string(root)

	return filepath.Walk(rootStr, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fn(path, info, err)
		}

		if info.IsDir() && path != rootStr {
			if !recursive || skipDirForGoList(path, info.Name()) {
				return filepath.SkipDir
			}
		}

		return fn(path, info, nil)
	})
}

func skipDirForGoList(path, name string) bool {
	if name == "vendor" || name == "testdata" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
		return true
	}

	_, err := os.Stat(filepath.Join(path, "go.mod"))

	return err == nil
}

// ReadFile loads file contents from disk.
func (a *LocalSourceFSAdapter) ReadFile(ctx context.Context, path m.Path) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return os.ReadFile(string(path))
}

// HashFile returns the SHA-256 hash of the file at the provided path.
func (a *LocalSourceFSAdapter) HashFile(ctx context.Context, path m.Path) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	f, err := os.Open(string(path))
	if err != nil {
		return "", err
	}

	defer func() {
		_ = f.Close()
	}()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// DetectTestFile finds the companion *_test.go file for the provided source path.
func (a *LocalSourceFSAdapter) DetectTestFile(ctx context.Context, sourcePath m.Path) (m.Path, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	source := string(sourcePath)
	if filepath.Ext(source) != ".go" {
		return "", nil
	}

	if strings.HasSuffix(source, "_test.go") {
		return "", nil
	}

	dir := filepath.Dir(source)
	base := strings.TrimSuffix(filepath.Base(source), ".go")
	testFile := filepath.Join(dir, base+"_test.go")

	if _, err := os.Stat(testFile); err == nil {
		return m.Path(testFile), nil
	} else if !os.IsNotExist(err) {
		return "", err
	}

	matches, err := filepath.Glob(filepath.Join(dir, "*_test.go"))
	if err != nil || len(matches) == 0 {
		return "", err
	}

	sort.Strings(matches)

	return m.Path(matches[0]), nil
}

// FileInfo returns os.FileInfo metadata for the given path.
func (a *LocalSourceFSAdapter) FileInfo(ctx context.Context, path m.Path) (os.FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return os.Stat(string(path))
}

// FindProjectRoot searches for go.mod file walking up the directory tree.
func (a *LocalSourceFSAdapter) FindProjectRoot(ctx context.Context, startPath m.Path) (m.Path, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	dir := filepath.Dir(string(startPath))

	for {
		goModPath := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			return m.Path(dir), nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found in any parent directory of %s", startPath)
		}

		dir = parent
	}
}

// CreateTempDir creates a temporary directory for mutation testing.
func (a *LocalSourceFSAdapter) CreateTempDir(ctx context.Context, pattern string) (m.Path, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	tmpDir, err := os.MkdirTemp("", pattern)
	if err != nil {
		return "", err
	}

	return m.Path(tmpDir), nil
}

// RemoveAll removes a directory and all its contents.
func (a *LocalSourceFSAdapter) RemoveAll(ctx context.Context, path m.Path) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	return os.RemoveAll(string(path))
}

// CopyDir recursively copies a directory tree.
func (a *LocalSourceFSAdapter) CopyDir(ctx context.Context, src, dst m.Path) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	return filepath.Walk(string(src), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(string(src), path)
		if err != nil {
			return err
		}

		if info.IsDir() {
			baseName := filepath.Base(path)
			if baseName == ".git" || baseName == "node_modules" {
				return filepath.SkipDir
			}
		}

		return a.copyEntry(path, filepath.Join(string(dst), relPath), info)
	})
}

func (a *LocalSourceFSAdapter) copyEntry(path, targetPath string, info os.FileInfo) error {
	switch {
	case info.IsDir():
		return os.MkdirAll(targetPath, info.Mode())
	case info.Mode()&os.ModeSymlink != 0:
		link, err := os.Readlink(path)
		if err != nil {
			return err
		}

		return os.Symlink(link, targetPath)
	case info.Mode().IsRegular():
		return a.copyFile(path, targetPath, info.Mode())
	default:
		return nil
	}
}

// copyFile copies a single file.
func (a *LocalSourceFSAdapter) copyFile(src, dst string, mode os.FileMode) error {
	// #nosec G304 - src is internal project file path, not user input
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}

	defer func() { _ = sourceFile.Close() }()

	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return err
	}

	// #nosec G304 - dst is internal destination path, not user input
	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}

	defer func() { _ = destFile.Close() }()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return err
	}

	return os.Chmod(dst, mode)
}

// WriteFile writes content to a file with the given permissions.
func (a *LocalSourceFSAdapter) WriteFile(ctx context.Context, path m.Path, content []byte, perm os.FileMode) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	return os.WriteFile(string(path), content, perm)
}

// RelPath returns the relative path from base to target.
func (a *LocalSourceFSAdapter) RelPath(ctx context.Context, base, target m.Path) (m.Path, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	rel, err := filepath.Rel(string(base), string(target))
	if err != nil {
		return "", err
	}

	return m.Path(rel), nil
}

// JoinPath joins path elements into a single path.
func (a *LocalSourceFSAdapter) JoinPath(ctx context.Context, elem ...string) m.Path {
	if err := ctx.Err(); err != nil {
		return ""
	}

	return m.Path(filepath.Join(elem...))
}

// dedupEmit wraps emit so each unique source (by origin path) is emitted once.
func dedupEmit(seen map[string]struct{}, emit func(rpt.Source) error) func(rpt.Source) error {
	return func(source rpt.Source) error {
		if source.Origin == nil {
			return nil
		}

		if _, exists := seen[source.Origin.FullPath]; exists {
			return nil
		}

		seen[source.Origin.FullPath] = struct{}{}

		return emit(source)
	}
}

func (a *LocalSourceFSAdapter) collectSourcesFromDir(ctx context.Context, rootPath string, recursive bool, ignoreRegexps []*regexp.Regexp, emit func(rpt.Source) error) error {
	return a.Walk(ctx, m.Path(rootPath), recursive, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		source, err := a.processFilePath(ctx, path, ignoreRegexps)
		if err != nil {
			if isInvalidSourceErr(err) {
				slog.Warn("Skipping source file that could not be parsed", "path", path, "error", err)

				return nil
			}

			return err
		}

		return emit(source)
	})
}

func normalizeRootPath(root string) (string, bool, error) {
	rootStr, recursive := parseRootPath(root)

	if strings.HasPrefix(rootStr, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", false, err
		}

		suffix := strings.TrimPrefix(rootStr, "~")
		suffix = strings.TrimPrefix(suffix, string(os.PathSeparator))
		rootStr = filepath.Join(home, suffix)
	}

	if rootStr == "" {
		rootStr = "."
	}

	abs, err := filepath.Abs(rootStr)
	if err != nil {
		return "", false, err
	}

	return abs, recursive, nil
}

func parseRootPath(rootStr string) (path string, recursive bool) {
	if len(rootStr) >= 4 && rootStr[len(rootStr)-4:] == "/..." {
		return rootStr[:len(rootStr)-4], true
	}

	return rootStr, false
}

// processFilePath builds the source for path, or returns a source without an
// origin when path is not a mutation candidate.
func (a *LocalSourceFSAdapter) processFilePath(ctx context.Context, path string, ignoreRegexps []*regexp.Regexp) (rpt.Source, error) {
	if !isCandidateSourcePath(path, ignoreRegexps) {
		return rpt.Source{}, nil
	}

	return a.buildSourceFromPath(ctx, path, ignoreRegexps)
}

func isCandidateSourcePath(path string, ignoreRegexps []*regexp.Regexp) bool {
	if filepath.Ext(path) != ".go" {
		return false
	}

	if strings.HasSuffix(path, "_test.go") {
		return false
	}

	return !shouldIgnorePath(path, ignoreRegexps)
}

func (a *LocalSourceFSAdapter) buildSourceFromPath(ctx context.Context, path string, ignoreRegexps []*regexp.Regexp) (rpt.Source, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return rpt.Source{}, err
	}

	projectRoot, rootErr := a.FindProjectRoot(ctx, m.Path(absPath))

	file, err := a.readAndParseSource(ctx, absPath)
	if err != nil {
		return rpt.Source{}, err
	}

	origin, err := a.buildOriginFile(ctx, absPath, projectRoot, rootErr)
	if err != nil {
		return rpt.Source{}, err
	}

	testFile := a.detectTestFile(ctx, m.Path(absPath), projectRoot, ignoreRegexps)

	packageName := file.Name.Name

	return rpt.Source{
		Origin:  origin,
		Test:    testFile,
		Package: &packageName,
	}, nil
}

func (a *LocalSourceFSAdapter) readAndParseSource(ctx context.Context, absPath string) (*ast.File, error) {
	src, err := a.ReadFile(ctx, m.Path(absPath))
	if err != nil {
		return nil, fmt.Errorf("%w: read source file: %w", errInvalidSource, err)
	}

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, absPath, src, parser.AllErrors)
	if err != nil {
		return nil, fmt.Errorf("%w: parse source file: %w", errInvalidSource, err)
	}

	if file.Name == nil {
		return nil, fmt.Errorf("%w: missing package name", errInvalidSource)
	}

	return file, nil
}

func (a *LocalSourceFSAdapter) buildOriginFile(ctx context.Context, absPath string, projectRoot m.Path, rootErr error) (*rpt.File, error) {
	originHash, err := a.HashFile(ctx, m.Path(absPath))
	if err != nil {
		return nil, err
	}

	origin := &rpt.File{FullPath: absPath, Hash: originHash}
	if rootErr == nil {
		if relPath, err := a.RelPath(ctx, projectRoot, m.Path(absPath)); err == nil {
			origin.ShortPath = string(relPath)
		}
	}

	return origin, nil
}

func (a *LocalSourceFSAdapter) detectTestFile(ctx context.Context, sourcePath m.Path, projectRoot m.Path, ignoreRegexps []*regexp.Regexp) *rpt.File {
	testPath := a.resolveTestPath(ctx, sourcePath, ignoreRegexps)
	if testPath == "" {
		return nil
	}

	file, err := a.buildTestFile(ctx, testPath, projectRoot)
	if err != nil {
		return nil
	}

	return file
}

func (a *LocalSourceFSAdapter) resolveTestPath(ctx context.Context, sourcePath m.Path, ignoreRegexps []*regexp.Regexp) m.Path {
	testPath, err := a.DetectTestFile(ctx, sourcePath)
	if err != nil || testPath == "" {
		return ""
	}

	if shouldIgnorePath(string(testPath), ignoreRegexps) {
		return ""
	}

	return testPath
}

func (a *LocalSourceFSAdapter) buildTestFile(ctx context.Context, testPath m.Path, projectRoot m.Path) (*rpt.File, error) {
	if err := a.validateGoFile(ctx, testPath); err != nil {
		return nil, err
	}

	testHash, err := a.HashFile(ctx, testPath)
	if err != nil {
		return nil, err
	}

	file := &rpt.File{FullPath: string(testPath), Hash: testHash}
	if projectRoot != "" {
		if relPath, err := a.RelPath(ctx, projectRoot, testPath); err == nil {
			file.ShortPath = string(relPath)
		}
	}

	return file, nil
}

func (a *LocalSourceFSAdapter) validateGoFile(ctx context.Context, path m.Path) error {
	src, err := a.ReadFile(ctx, path)
	if err != nil {
		return err
	}

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, string(path), src, parser.AllErrors)
	if err != nil {
		return err
	}

	if file == nil || file.Name == nil {
		return fmt.Errorf("invalid go file")
	}

	return nil
}

var errInvalidSource = errors.New("invalid source file")

func compileIgnoreRegexps(patterns []string) ([]*regexp.Regexp, error) {
	if len(patterns) == 0 {
		return nil, nil
	}

	regexps := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}

		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid ignore pattern %q: %w", pattern, err)
		}

		regexps = append(regexps, re)
	}

	return regexps, nil
}

func shouldIgnorePath(path string, ignoreRegexps []*regexp.Regexp) bool {
	if len(ignoreRegexps) == 0 {
		return false
	}

	candidates := []string{path, filepath.Base(path)}
	if rel, ok := relativeToCwd(path); ok {
		candidates = append(candidates, rel)
	}

	for _, re := range ignoreRegexps {
		if slices.ContainsFunc(candidates, re.MatchString) {
			return true
		}
	}

	return false
}

func relativeToCwd(path string) (string, bool) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", false
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}

	rel, err := filepath.Rel(cwd, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", false
	}

	return filepath.ToSlash(rel), true
}

func isInvalidSourceErr(err error) bool {
	return errors.Is(err, errInvalidSource)
}
