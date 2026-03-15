package adapter

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	m "gooze.dev/pkg/gooze/internal/model"
)

type FilesAdapter interface {
	// Get scans the specified root directories for Go source files, applies exclusion rules, and emits found sources through a channel.
	Get(ctx context.Context, rootDirs []m.Path, parallel int, exclude ...string) (<-chan m.Source, error)
	
	// ReadFile loads a file from disk and returns its contents.
	ReadFile(ctx context.Context, path m.Path) ([]byte, error)

}

type filesAdapter struct {
	events m.Events
}

func NewFilesAdapter(events m.Events) FilesAdapter {
	return &filesAdapter{
		events: events,
	}
}

func (a *filesAdapter) Get(ctx context.Context, rootDirs []m.Path, parallel int, exclude ...string) (<-chan m.Source, error) {
	ch := make(chan m.Source, parallel)

	if len(rootDirs) == 0 {
		close(ch)
		return ch, nil
	}

	ignoreRegexps, err := compileIgnoreRegexps(exclude)
	if err != nil {
		close(ch)
		return ch, err
	}

	go func() {
		defer close(ch)
		a.events.StartScanningPaths()

		seen := make(map[string]struct{})

		for _, rootDir := range rootDirs {
			if ctx.Err() != nil {
				return
			}

			rootPath, recursive, err := normalizeRootPath(string(rootDir))
			if err != nil {
				slog.Error("Failed to normalize root path", "path", rootDir, "error", err)
				a.events.Error(fmt.Errorf("normalize root path: %w", err))
				continue
			}

			info, err := os.Stat(rootPath)
			if err != nil {
				slog.Error("Failed to stat path", "path", rootPath, "error", err)
				a.events.Error(fmt.Errorf("stat path: %w", err))
				continue
			}

			slog.Debug("Scanning path", "path", rootPath, "recursive", recursive)
			a.events.ScanningPath(m.Path(rootPath))

			if !info.IsDir() {
				// Single file path
				source, ok, err := a.processFilePath(ctx, rootPath, ignoreRegexps)
				if err != nil && !isInvalidSourceErr(err) {
					slog.Error("Failed to process file", "path", rootPath, "error", err)
					a.events.Error(fmt.Errorf("process file path: %w", err))
					continue
				}

				if ok {
					originPath := string(source.Origin.FullPath)
					if _, exists := seen[originPath]; !exists {
						seen[originPath] = struct{}{}
						select {
						case ch <- source:
						case <-ctx.Done():
							return
						}
					}
				}

				continue
			}

			// Directory path — walk it
			err = a.walkDir(ctx, rootPath, recursive, ignoreRegexps, seen, ch)
			if err != nil && !errors.Is(err, context.Canceled) {
				slog.Error("Failed to walk directory", "path", rootPath, "error", err)
				a.events.Error(fmt.Errorf("walk directory: %w", err))
				continue
			}

			slog.Debug("Finished scanning path", "path", rootPath)
		}
		a.events.FinishScanningPaths()
	}()

	return ch, nil
}

// ReadFile loads file contents from disk.
func (a *filesAdapter) ReadFile(ctx context.Context, path m.Path) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return os.ReadFile(string(path))
}


func (a *filesAdapter) walkDir(ctx context.Context, rootPath string, recursive bool, ignoreRegexps []*regexp.Regexp, seen map[string]struct{}, ch chan<- m.Source) error {
	return filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files with errors
		}

		if ctx.Err() != nil {
			a.events.Error(fmt.Errorf("context canceled: %w", ctx.Err()))
			return context.Canceled
		}

		// Skip directories
		if info.IsDir() {
			// If non-recursive and not the root, skip
			if !recursive && path != rootPath {
				return filepath.SkipDir
			}
			return nil
		}

		// Only process .go files that are not test files
		if !isCandidateSourcePath(path, ignoreRegexps) {
			return nil
		}

		source, ok, err := a.processFilePath(ctx, path, ignoreRegexps)
		if err != nil {
			if !isInvalidSourceErr(err) {
				slog.Error("Failed to process file", "path", path, "error", err)
			}
			return nil
		}

		if !ok {
			return nil
		}

		originPath := string(source.Origin.FullPath)
		if _, exists := seen[originPath]; exists {
			return nil // Already seen
		}

		seen[originPath] = struct{}{}

		select {
		case ch <- source:
		case <-ctx.Done():
			return context.Canceled
		}

		return nil
	})
}

func (a *filesAdapter) processFilePath(ctx context.Context, path string, ignoreRegexps []*regexp.Regexp) (m.Source, bool, error) {
	if !isCandidateSourcePath(path, ignoreRegexps) {
		return m.Source{}, false, nil
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return m.Source{}, false, err
	}

	// Parse and validate Go file
	src, err := os.ReadFile(absPath)
	if err != nil {
		return m.Source{}, false, fmt.Errorf("read source file: %w", err)
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, absPath, src, parser.AllErrors)
	if err != nil {
		return m.Source{}, false, fmt.Errorf("%w: parse source file: %w", errInvalidSource, err)
	}

	if file == nil || file.Name == nil {
		return m.Source{}, false, fmt.Errorf("%w: missing package name", errInvalidSource)
	}

	packageName := file.Name.Name

	// Compute hash
	originHash := fmt.Sprintf("%x", sha256.Sum256(src))

	// Find project root (go.mod) for short path computation
	projectRoot := a.findProjectRoot(absPath)

	// Build Origin file with both full and short paths
	origin := &m.File{
		FullPath: m.Path(absPath),
		Hash:     originHash,
	}
	if projectRoot != "" {
		if shortPath, err := filepath.Rel(projectRoot, absPath); err == nil {
			origin.ShortPath = m.Path(shortPath)
		}
	}

	// Emit event for found source file
	slog.Debug("Found source file", "path", absPath)

	// Try to find test file
	var testFile *m.File
	testPath := a.resolveTestPath(ctx, m.Path(absPath), ignoreRegexps)
	if testPath != "" {
		testFile = a.buildTestFile(testPath, projectRoot)
	}

	source := m.Source{
		Origin:  origin,
		Test:    testFile,
		Package: &packageName,
	}
	a.events.PairFound(source)
	return source, true, nil
}

func (a *filesAdapter) resolveTestPath(ctx context.Context, sourcePath m.Path, ignoreRegexps []*regexp.Regexp) m.Path {
	source := string(sourcePath)
	if filepath.Ext(source) != ".go" {
		return ""
	}

	if strings.HasSuffix(source, "_test.go") {
		return ""
	}

	base := strings.TrimSuffix(filepath.Base(source), ".go")
	testPath := filepath.Join(filepath.Dir(source), base+"_test.go")

	// Check if file exists
	if _, err := os.Stat(testPath); err != nil {
		return ""
	}

	// Check if it should be ignored
	if shouldIgnorePath(testPath, ignoreRegexps) {
		return ""
	}

	return m.Path(testPath)
}

func (a *filesAdapter) buildTestFile(testPath m.Path, projectRoot string) *m.File {
	// Validate that it's a valid Go file
	src, err := os.ReadFile(string(testPath))
	if err != nil {
		return nil
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, string(testPath), src, parser.AllErrors)
	if err != nil || file == nil || file.Name == nil {
		return nil
	}

	testHash := fmt.Sprintf("%x", sha256.Sum256(src))
	testFile := &m.File{
		FullPath: testPath,
		Hash:     testHash,
	}

	// Compute short path if project root was found
	if projectRoot != "" {
		if shortPath, err := filepath.Rel(projectRoot, string(testPath)); err == nil {
			testFile.ShortPath = m.Path(shortPath)
		}
	}

	return testFile
}

func (a *filesAdapter) findProjectRoot(filePath string) string {
	dir := filepath.Dir(filePath)

	for {
		goModPath := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root directory without finding go.mod
			return ""
		}

		dir = parent
	}
}