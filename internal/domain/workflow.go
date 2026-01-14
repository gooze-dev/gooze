package domain

import (
	"fmt"
	"go/token"
	"os"
	"strings"

	"github.com/mouse-blink/gooze/internal/adapter"
	m "github.com/mouse-blink/gooze/internal/model"
)

const goFileExt = ".go"

// Workflow defines the interface for mutation testing operations.
type Workflow interface {
	GetSources(roots ...m.Path) ([]m.Source, error)
	GenerateMutations(sources m.Source, mutationType ...m.MutationType) ([]m.Mutation, error)
	EstimateMutations(sources m.Source, mutationType ...m.MutationType) (int, error)
	TestMutation(sources m.Source, mutation m.Mutation) (m.Report, error)
}

type workflow struct {
	fsAdapter   adapter.SourceFSAdapter
	goAdapter   adapter.GoFileAdapter
	testAdapter adapter.TestRunnerAdapter
	mutagen     Mutagen
	orch        Orchestrator
}

// NewWorkflow creates a new Workflow instance with the provided adapters.
func NewWorkflow(fsAdapter adapter.SourceFSAdapter, goAdapter adapter.GoFileAdapter, testAdapter adapter.TestRunnerAdapter) Workflow {
	return &workflow{
		fsAdapter:   fsAdapter,
		goAdapter:   goAdapter,
		testAdapter: testAdapter,
		mutagen:     NewMutagen(),
		orch:        NewOrchestrator(fsAdapter, testAdapter),
	}
}

// GetSources walks directory trees and identifies code scopes for mutation testing.
// Supports multiple paths and ./... suffix for recursive scanning.
// It distinguishes between:
// - Global scope (const, var, type declarations) - for mutations like boolean literals, numbers
// - Init functions - for all mutation types
// - Regular functions - for function-specific mutations.
func (w *workflow) GetSources(roots ...m.Path) ([]m.Source, error) {
	if len(roots) == 0 {
		return []m.Source{}, nil
	}

	seen := make(map[string]bool)

	var allSources []m.Source

	for _, root := range roots {
		sources, err := w.scanPath(root)
		if err != nil {
			return nil, err
		}

		for _, source := range sources {
			absPath := string(source.Origin)
			if !seen[absPath] {
				seen[absPath] = true

				allSources = append(allSources, source)
			}
		}
	}

	return allSources, nil
}

// GenerateMutations delegates to the mutagen for pure mutation generation.
func (w *workflow) GenerateMutations(source m.Source, mutationTypes ...m.MutationType) ([]m.Mutation, error) {
	return w.mutagen.GenerateMutations(source, mutationTypes...)
}

// EstimateMutations delegates to the mutagen for mutation estimation.
func (w *workflow) EstimateMutations(source m.Source, mutationTypes ...m.MutationType) (int, error) {
	return w.mutagen.EstimateMutations(source, mutationTypes...)
}

// scanPath scans a single path (with optional /... suffix) for Go source files.
func (w *workflow) scanPath(root m.Path) ([]m.Source, error) {
	rootStr, recursive := parseRootPath(string(root))

	if _, err := w.fsAdapter.FileInfo(m.Path(rootStr)); err != nil {
		return nil, fmt.Errorf("root path error: %w", err)
	}

	var sources []m.Source

	fset := token.NewFileSet()

	err := w.fsAdapter.Walk(m.Path(rootStr), recursive, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			if !recursive && path != rootStr {
				return fmt.Errorf("skip directory") // Skip subdirectories if not recursive
			}

			return nil
		}

		source, shouldInclude, processErr := w.processFile(m.Path(path), fset)
		if processErr != nil {
			return processErr
		}

		if shouldInclude {
			sources = append(sources, source)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return sources, nil
}

// parseRootPath extracts the root path and recursive flag from a path string.
func parseRootPath(rootStr string) (path string, recursive bool) {
	if len(rootStr) >= 4 && rootStr[len(rootStr)-4:] == "/..." {
		return rootStr[:len(rootStr)-4], true
	}

	return rootStr, false
}

// processFile parses and extracts scopes from a single Go file.
func (w *workflow) processFile(path m.Path, fset *token.FileSet) (m.Source, bool, error) {
	pathStr := string(path)

	// Skip non-Go files
	if !strings.HasSuffix(pathStr, goFileExt) {
		return m.Source{}, false, nil
	}

	// Skip test files (e.g., *_test.go)
	if strings.HasSuffix(pathStr, "_test.go") {
		return m.Source{}, false, nil
	}

	src, err := w.fsAdapter.ReadFile(path)
	if err != nil {
		return m.Source{}, false, nil //nolint:nilerr // Intentionally skip unreadable files
	}

	file, err := w.goAdapter.Parse(fset, pathStr, src)
	if err != nil {
		return m.Source{}, false, nil //nolint:nilerr // Intentionally skip unparsable files
	}

	scopes := w.goAdapter.ExtractScopes(fset, file)
	if len(scopes) == 0 {
		return m.Source{}, false, nil
	}

	functionLines := w.goAdapter.FunctionLines(scopes)
	hasGlobals := hasGlobalScopes(scopes)

	// Include only if it has functions/init or global declarations
	if len(functionLines) == 0 && !hasGlobals {
		return m.Source{}, false, nil
	}

	hash, err := w.fsAdapter.HashFile(path)
	if err != nil {
		return m.Source{}, false, fmt.Errorf("hash error for %s: %w", path, err)
	}

	// Detect corresponding test file
	testFile, _ := w.fsAdapter.DetectTestFile(path)

	source := m.Source{
		Hash:   hash,
		Origin: path,
		Test:   testFile,
		Lines:  functionLines,
		Scopes: scopes,
	}

	return source, true, nil
}

// hasGlobalScopes checks if there are any global const/var scopes.
func hasGlobalScopes(scopes []m.CodeScope) bool {
	for _, scope := range scopes {
		if scope.Type == m.ScopeGlobal {
			return true
		}
	}

	return false
}

// TestMutation applies a mutation to source code and runs tests to check if the mutation is detected.
func (w *workflow) TestMutation(source m.Source, mutation m.Mutation) (m.Report, error) {
	return w.orch.TestMutation(source, mutation)
}
