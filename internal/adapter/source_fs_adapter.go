// Package adapter contains UI and infrastructure adapters for the Gooze CLI.
package adapter

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strings"

	m "github.com/mouse-blink/gooze/internal/model"
)

// SourceFSAdapter abstracts filesystem-specific operations that the domain layer
// relies on when scanning user projects. It intentionally hides direct `os`
// access so the workflow logic can be tested without touching the disk.
//
//nolint:interfacebloat // A richer interface keeps workflow logic decoupled from os/fs.
type SourceFSAdapter interface {
	Get(root []m.Path) ([]m.Source, error)

	// Walk traverses the provided root path. When recursive is false the
	// implementation should limit itself to the root directory (no sub-dirs).
	Walk(root m.Path, recursive bool, fn FilepathWalkFunc) error

	// ReadFile loads a file from disk and returns its contents.
	ReadFile(path m.Path) ([]byte, error)

	// HashFile returns a stable fingerprint (e.g. SHA-256) for the file at path.
	HashFile(path m.Path) (string, error)

	// DetectTestFile attempts to find a Go test file that matches the provided
	// source file. This allows the domain to auto-link source/test pairs.
	DetectTestFile(sourcePath m.Path) (m.Path, error)

	// FileInfo returns metadata for a path so the domain can check existence or
	// distinguish between files and directories when necessary.
	FileInfo(path m.Path) (os.FileInfo, error)

	// FindProjectRoot searches for go.mod file walking up the directory tree.
	FindProjectRoot(startPath m.Path) (m.Path, error)

	// CreateTempDir creates a temporary directory for mutation testing.
	CreateTempDir(pattern string) (m.Path, error)

	// RemoveAll removes a directory and all its contents.
	RemoveAll(path m.Path) error

	// CopyDir recursively copies a directory tree.
	CopyDir(src, dst m.Path) error

	// WriteFile writes content to a file with the given permissions.
	WriteFile(path m.Path, content []byte, perm os.FileMode) error

	// RelPath returns the relative path from base to target.
	RelPath(base, target m.Path) (m.Path, error)

	// JoinPath joins path elements into a single path.
	JoinPath(elem ...string) m.Path
}

// FilepathWalkFunc mirrors the callback shape used by filepath.WalkDir. It is
// defined here to avoid leaking the standard-library type directly into the
// domain layer.
type FilepathWalkFunc func(path string, info os.FileInfo, err error) error

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
func (a *LocalSourceFSAdapter) Get(roots []m.Path) ([]m.Source, error) {
	if len(roots) == 0 {
		return []m.Source{}, nil
	}

	seen := make(map[string]struct{})
	sources := make([]m.Source, 0, len(roots))

	for _, root := range roots {
		rootPath, recursive, err := normalizeRootPath(string(root))
		if err != nil {
			return nil, err
		}

		info, err := a.FileInfo(m.Path(rootPath))
		if err != nil {
			return nil, fmt.Errorf("root path error: %w", err)
		}

		if !info.IsDir() {
			source, ok, err := a.processFilePath(rootPath)
			if err != nil {
				if isInvalidSourceErr(err) {
					continue
				}

				return nil, err
			}

			if ok {
				addSourceIfNew(&sources, seen, source)
			}

			continue
		}

		if err := a.collectSourcesFromDir(rootPath, recursive, seen, &sources); err != nil {
			return nil, err
		}
	}

	return sources, nil
}

// Walk iterates over files under root, optionally descending into subdirectories.
func (a *LocalSourceFSAdapter) Walk(root m.Path, recursive bool, fn FilepathWalkFunc) error {
	rootStr := string(root)

	return filepath.Walk(rootStr, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fn(path, info, err)
		}

		if info.IsDir() && !recursive && path != rootStr {
			return filepath.SkipDir
		}

		return fn(path, info, nil)
	})
}

// ReadFile loads file contents from disk.
func (a *LocalSourceFSAdapter) ReadFile(path m.Path) ([]byte, error) {
	return os.ReadFile(string(path))
}

// HashFile returns the SHA-256 hash of the file at the provided path.
func (a *LocalSourceFSAdapter) HashFile(path m.Path) (string, error) {
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
func (a *LocalSourceFSAdapter) DetectTestFile(sourcePath m.Path) (m.Path, error) {
	source := string(sourcePath)
	if filepath.Ext(source) != ".go" {
		return "", nil
	}

	if strings.HasSuffix(source, "_test.go") {
		return "", nil
	}

	base := strings.TrimSuffix(filepath.Base(source), ".go")
	testFile := filepath.Join(filepath.Dir(source), base+"_test.go")

	if _, err := os.Stat(testFile); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}

		return "", err
	}

	return m.Path(testFile), nil
}

// FileInfo returns os.FileInfo metadata for the given path.
func (a *LocalSourceFSAdapter) FileInfo(path m.Path) (os.FileInfo, error) {
	return os.Stat(string(path))
}

// FindProjectRoot searches for go.mod file walking up the directory tree.
func (a *LocalSourceFSAdapter) FindProjectRoot(startPath m.Path) (m.Path, error) {
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
func (a *LocalSourceFSAdapter) CreateTempDir(pattern string) (m.Path, error) {
	tmpDir, err := os.MkdirTemp("", pattern)
	if err != nil {
		return "", err
	}

	return m.Path(tmpDir), nil
}

// RemoveAll removes a directory and all its contents.
func (a *LocalSourceFSAdapter) RemoveAll(path m.Path) error {
	return os.RemoveAll(string(path))
}

// CopyDir recursively copies a directory tree.
func (a *LocalSourceFSAdapter) CopyDir(src, dst m.Path) error {
	return filepath.Walk(string(src), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(string(src), path)
		if err != nil {
			return err
		}

		// Skip common directories that don't need to be copied
		if info.IsDir() {
			baseName := filepath.Base(path)
			if baseName == ".git" || baseName == "vendor" || baseName == "node_modules" {
				return filepath.SkipDir
			}
		}

		targetPath := filepath.Join(string(dst), relPath)

		if info.IsDir() {
			return os.MkdirAll(targetPath, info.Mode())
		}

		return a.copyFile(path, targetPath, info.Mode())
	})
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
func (a *LocalSourceFSAdapter) WriteFile(path m.Path, content []byte, perm os.FileMode) error {
	return os.WriteFile(string(path), content, perm)
}

// RelPath returns the relative path from base to target.
func (a *LocalSourceFSAdapter) RelPath(base, target m.Path) (m.Path, error) {
	rel, err := filepath.Rel(string(base), string(target))
	if err != nil {
		return "", err
	}

	return m.Path(rel), nil
}

// JoinPath joins path elements into a single path.
func (a *LocalSourceFSAdapter) JoinPath(elem ...string) m.Path {
	return m.Path(filepath.Join(elem...))
}

func addSourceIfNew(sources *[]m.Source, seen map[string]struct{}, source m.Source) {
	if source.Origin == nil {
		return
	}

	if _, exists := seen[string(source.Origin.Path)]; exists {
		return
	}

	seen[string(source.Origin.Path)] = struct{}{}
	*sources = append(*sources, source)
}

func (a *LocalSourceFSAdapter) collectSourcesFromDir(rootPath string, recursive bool, seen map[string]struct{}, sources *[]m.Source) error {
	return a.Walk(m.Path(rootPath), recursive, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		source, ok, err := a.processFilePath(path)
		if err != nil {
			if isInvalidSourceErr(err) {
				return nil
			}

			return err
		}

		if !ok {
			return nil
		}

		addSourceIfNew(sources, seen, source)

		return nil
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

func (a *LocalSourceFSAdapter) processFilePath(path string) (m.Source, bool, error) {
	if filepath.Ext(path) != ".go" {
		return m.Source{}, false, nil
	}

	if strings.HasSuffix(path, "_test.go") {
		return m.Source{}, false, nil
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return m.Source{}, false, err
	}

	src, err := a.ReadFile(m.Path(absPath))
	if err != nil {
		return m.Source{}, false, fmt.Errorf("%w: read source file: %w", errInvalidSource, err)
	}

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, absPath, src, parser.AllErrors)
	if err != nil {
		return m.Source{}, false, fmt.Errorf("%w: parse source file: %w", errInvalidSource, err)
	}

	if file.Name == nil {
		return m.Source{}, false, fmt.Errorf("%w: missing package name", errInvalidSource)
	}

	originHash, err := a.HashFile(m.Path(absPath))
	if err != nil {
		return m.Source{}, false, err
	}

	origin := &m.File{Path: m.Path(absPath), Hash: originHash}

	testFile := a.detectTestFile(m.Path(absPath))

	packageName := file.Name.Name

	return m.Source{
		Origin:  origin,
		Test:    testFile,
		Package: &packageName,
	}, true, nil
}

func (a *LocalSourceFSAdapter) detectTestFile(sourcePath m.Path) *m.File {
	testPath, err := a.DetectTestFile(sourcePath)
	if err != nil || testPath == "" {
		return nil
	}

	testSrc, err := a.ReadFile(testPath)
	if err != nil {
		return nil
	}

	testFset := token.NewFileSet()

	testAST, err := parser.ParseFile(testFset, string(testPath), testSrc, parser.AllErrors)
	if err != nil || testAST == nil || testAST.Name == nil {
		return nil
	}

	testHash, err := a.HashFile(testPath)
	if err != nil {
		return nil
	}

	return &m.File{Path: testPath, Hash: testHash}
}

var errInvalidSource = errors.New("invalid source file")

func isInvalidSourceErr(err error) bool {
	return errors.Is(err, errInvalidSource)
}
