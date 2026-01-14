package domain

import (
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mouse-blink/gooze/internal/adapter"
	m "github.com/mouse-blink/gooze/internal/model"
)

func TestGetSources(t *testing.T) {
	t.Run("detects functions in files", func(t *testing.T) {
		root := t.TempDir()

		// Copy basic example (has main function) and types example (only types)
		if err := copyExampleDir("../../examples/basic", root); err != nil {
			t.Fatalf("failed to copy basic example: %v", err)
		}

		wf := NewWorkflow(adapter.NewLocalSourceFSAdapter(), adapter.NewLocalGoFileAdapter(), adapter.NewLocalTestRunnerAdapter())
		sources, err := wf.GetSources(m.Path(filepath.Join(root, "...")))
		if err != nil {
			t.Fatalf("GetSources error: %v", err)
		}
		if len(sources) == 0 {
			t.Fatalf("expected sources, got 0")
		}

		mainPath := filepath.Join(root, "main.go")
		lines, ok := findLinesFor(sources, mainPath)
		if !ok {
			t.Fatalf("expected to find source for %s", mainPath)
		}
		// main function should be detected
		if len(lines) == 0 {
			t.Errorf("expected function lines in %s, got %v", mainPath, lines)
		}
		// Ensure type.go (no functions) is not included if present
		if _, present := findLinesFor(sources, filepath.Join(root, "type.go")); present {
			t.Errorf("did not expect type.go to be reported (contains no functions)")
		}
	})

	t.Run("excludes *_test.go files from sources", func(t *testing.T) {
		root := t.TempDir()

		// Copy example project which includes a test file
		srcDir := "../../examples/basic"
		if err := copyExampleDir(srcDir, root); err != nil {
			t.Fatalf("failed to copy example: %v", err)
		}

		wf := NewWorkflow(adapter.NewLocalSourceFSAdapter(), adapter.NewLocalGoFileAdapter(), adapter.NewLocalTestRunnerAdapter())
		sources, err := wf.GetSources(m.Path(root))
		if err != nil {
			t.Fatalf("GetSources error: %v", err)
		}
		if len(sources) == 0 {
			t.Fatalf("expected at least one source")
		}

		// Ensure no test files are included
		for _, s := range sources {
			base := filepath.Base(string(s.Origin))
			if strings.HasSuffix(base, "_test.go") {
				t.Errorf("should not include test files: %s", s.Origin)
			}
		}

		// Ensure main.go is present
		mainPath := filepath.Join(root, "main.go")
		if _, ok := findSourceFor(sources, mainPath); !ok {
			t.Errorf("expected to include main.go in sources")
		}
	})

	t.Run("automatically detects test files", func(t *testing.T) {
		root := t.TempDir()

		// Copy basic example and add test file
		if err := copyExampleDir("../../examples/basic", root); err != nil {
			t.Fatalf("failed to copy example: %v", err)
		}

		// Create corresponding test file
		writeFile(t, filepath.Join(root, "main_test.go"), `package main

import "testing"

func TestMain(t *testing.T) {}
`)

		wf := NewWorkflow(adapter.NewLocalSourceFSAdapter(), adapter.NewLocalGoFileAdapter(), adapter.NewLocalTestRunnerAdapter())
		sources, err := wf.GetSources(m.Path(root))
		if err != nil {
			t.Fatalf("GetSources error: %v", err)
		}

		if len(sources) == 0 {
			t.Fatalf("expected at least one source")
		}

		// Find main.go source
		var mainSource m.Source
		found := false
		for _, s := range sources {
			if filepath.Base(string(s.Origin)) == "main.go" {
				mainSource = s
				found = true
				break
			}
		}

		if !found {
			t.Fatalf("main.go not found in sources")
		}

		// Check that test file was automatically detected
		if mainSource.Test == "" {
			t.Errorf("expected Test field to be set, got empty")
		}

		expectedTestPath := filepath.Join(root, "main_test.go")
		if string(mainSource.Test) != expectedTestPath {
			t.Errorf("expected Test = %s, got %s", expectedTestPath, mainSource.Test)
		}
	})

	t.Run("no test file when test does not exist", func(t *testing.T) {
		root := t.TempDir()

		// Copy basic example without test file
		if err := copyExampleDir("../../examples/basic", root); err != nil {
			t.Fatalf("failed to copy example: %v", err)
		}

		// Remove test file
		os.Remove(filepath.Join(root, "main_test.go"))

		wf := NewWorkflow(adapter.NewLocalSourceFSAdapter(), adapter.NewLocalGoFileAdapter(), adapter.NewLocalTestRunnerAdapter())
		sources, err := wf.GetSources(m.Path(root))
		if err != nil {
			t.Fatalf("GetSources error: %v", err)
		}

		if len(sources) == 0 {
			t.Fatalf("expected at least one source")
		}

		// Check that test file is empty when not found
		if sources[0].Test != "" {
			t.Errorf("expected Test field to be empty, got %s", sources[0].Test)
		}
	})

	t.Run("excludes files without functions", func(t *testing.T) {
		// Use nofunc example which has only type declarations
		root := "../../examples/nofunc"

		wf := NewWorkflow(adapter.NewLocalSourceFSAdapter(), adapter.NewLocalGoFileAdapter(), adapter.NewLocalTestRunnerAdapter())
		sources, err := wf.GetSources(m.Path(root))
		if err != nil {
			t.Fatalf("GetSources error: %v", err)
		}
		if len(sources) != 0 {
			t.Fatalf("expected 0 sources, got %d", len(sources))
		}
	})

	t.Run("walks nested directories with ./... pattern", func(t *testing.T) {
		// Use examples/nested which has sub/child.go structure
		root := "../../examples/nested"

		wf := NewWorkflow(adapter.NewLocalSourceFSAdapter(), adapter.NewLocalGoFileAdapter(), adapter.NewLocalTestRunnerAdapter())
		// Use ./... pattern for recursive scanning
		sources, err := wf.GetSources(m.Path(root + "/..."))
		if err != nil {
			t.Fatalf("GetSources error: %v", err)
		}
		childPath := filepath.Join(root, "sub", "child.go")
		if _, ok := findLinesFor(sources, childPath); !ok {
			t.Fatalf("expected to find nested source %s", childPath)
		}
	})

	t.Run("nonexistent root returns error", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "no_such_dir")
		wf := NewWorkflow(adapter.NewLocalSourceFSAdapter(), adapter.NewLocalGoFileAdapter(), adapter.NewLocalTestRunnerAdapter())
		_, err := wf.GetSources(m.Path(root))
		if err == nil {
			t.Fatalf("expected error for nonexistent root")
		}
	})

	t.Run("empty directory returns no sources", func(t *testing.T) {
		root := t.TempDir()
		wf := NewWorkflow(adapter.NewLocalSourceFSAdapter(), adapter.NewLocalGoFileAdapter(), adapter.NewLocalTestRunnerAdapter())
		sources, err := wf.GetSources(m.Path(root))
		if err != nil {
			t.Fatalf("GetSources error: %v", err)
		}
		if len(sources) != 0 {
			t.Fatalf("expected 0 sources in empty dir, got %d", len(sources))
		}
	})

	t.Run("invalid Go file is silently skipped", func(t *testing.T) {
		// Use examples/invalid which has broken.go and copy basic for valid file
		root := t.TempDir()
		if err := copyExampleDir("../../examples/invalid", root); err != nil {
			t.Fatalf("failed to copy invalid example: %v", err)
		}
		// Copy a valid file from basic example
		basicContent, err := os.ReadFile("../../examples/basic/main.go")
		if err != nil {
			t.Fatalf("failed to read basic/main.go: %v", err)
		}
		if err := os.WriteFile(filepath.Join(root, "good.go"), basicContent, 0o644); err != nil {
			t.Fatalf("failed to write good.go: %v", err)
		}

		wf := NewWorkflow(adapter.NewLocalSourceFSAdapter(), adapter.NewLocalGoFileAdapter(), adapter.NewLocalTestRunnerAdapter())
		sources, err := wf.GetSources(m.Path(root))
		if err != nil {
			t.Fatalf("GetSources error: %v", err)
		}
		// Should skip broken.go and only include good.go
		if len(sources) != 1 {
			t.Fatalf("expected 1 source (good.go), got %d", len(sources))
		}
		goodPath := filepath.Join(root, "good.go")
		if _, ok := findSourceFor(sources, goodPath); !ok {
			t.Errorf("expected to find good.go")
		}
	})

	t.Run("detects global constants with proper scope", func(t *testing.T) {
		root := "../../examples/constants"

		wf := NewWorkflow(adapter.NewLocalSourceFSAdapter(), adapter.NewLocalGoFileAdapter(), adapter.NewLocalTestRunnerAdapter())
		sources, err := wf.GetSources(m.Path(root))
		if err != nil {
			t.Fatalf("GetSources error: %v", err)
		}

		// Should detect file with constants even without functions
		if len(sources) == 0 {
			t.Fatalf("expected to find source with global constants")
		}

		constPath := filepath.Join(root, "main.go")
		source, ok := findSourceFor(sources, constPath)
		if !ok {
			t.Fatalf("expected to find source for %s", constPath)
		}

		// Should have global scopes for constants
		globalScopes := filterScopesByType(source.Scopes, m.ScopeGlobal)
		if len(globalScopes) != 2 {
			t.Errorf("expected 2 global scopes (MaxRetries, Enabled), got %d", len(globalScopes))
		}

		// Verify scope details
		if !hasScopeWithName(globalScopes, "MaxRetries") {
			t.Errorf("expected global scope for MaxRetries")
		}
		if !hasScopeWithName(globalScopes, "Enabled") {
			t.Errorf("expected global scope for Enabled")
		}
	})

	t.Run("detects global variables with proper scope", func(t *testing.T) {
		root := "../../examples/variables"

		wf := NewWorkflow(adapter.NewLocalSourceFSAdapter(), adapter.NewLocalGoFileAdapter(), adapter.NewLocalTestRunnerAdapter())
		sources, err := wf.GetSources(m.Path(root))
		if err != nil {
			t.Fatalf("GetSources error: %v", err)
		}

		if len(sources) == 0 {
			t.Fatalf("expected to find source with global variables")
		}

		varPath := filepath.Join(root, "main.go")
		source, ok := findSourceFor(sources, varPath)
		if !ok {
			t.Fatalf("expected to find source for %s", varPath)
		}

		globalScopes := filterScopesByType(source.Scopes, m.ScopeGlobal)
		if len(globalScopes) != 2 {
			t.Errorf("expected 2 global scopes, got %d", len(globalScopes))
		}
	})

	t.Run("detects init function with ScopeInit type", func(t *testing.T) {
		root := "../../examples/initfunc"

		wf := NewWorkflow(adapter.NewLocalSourceFSAdapter(), adapter.NewLocalGoFileAdapter(), adapter.NewLocalTestRunnerAdapter())
		sources, err := wf.GetSources(m.Path(root))
		if err != nil {
			t.Fatalf("GetSources error: %v", err)
		}

		if len(sources) == 0 {
			t.Fatalf("expected to find source with init function")
		}

		initPath := filepath.Join(root, "main.go")
		source, ok := findSourceFor(sources, initPath)
		if !ok {
			t.Fatalf("expected to find source for %s", initPath)
		}

		initScopes := filterScopesByType(source.Scopes, m.ScopeInit)
		if len(initScopes) != 1 {
			t.Errorf("expected 1 init scope, got %d", len(initScopes))
		}

		if len(initScopes) > 0 && initScopes[0].Name != "init" {
			t.Errorf("expected init scope name to be 'init', got %s", initScopes[0].Name)
		}
	})

	t.Run("detects mixed scopes in same file", func(t *testing.T) {
		root := "../../examples/mixed"

		wf := NewWorkflow(adapter.NewLocalSourceFSAdapter(), adapter.NewLocalGoFileAdapter(), adapter.NewLocalTestRunnerAdapter())
		sources, err := wf.GetSources(m.Path(root))
		if err != nil {
			t.Fatalf("GetSources error: %v", err)
		}

		if len(sources) == 0 {
			t.Fatalf("expected to find source with mixed scopes")
		}

		mixedPath := filepath.Join(root, "main.go")
		source, ok := findSourceFor(sources, mixedPath)
		if !ok {
			t.Fatalf("expected to find source for %s", mixedPath)
		}

		// Should have all three scope types
		globalScopes := filterScopesByType(source.Scopes, m.ScopeGlobal)
		initScopes := filterScopesByType(source.Scopes, m.ScopeInit)
		funcScopes := filterScopesByType(source.Scopes, m.ScopeFunction)

		if len(globalScopes) != 2 {
			t.Errorf("expected 2 global scopes (Pi, counter), got %d", len(globalScopes))
		}
		if len(initScopes) != 1 {
			t.Errorf("expected 1 init scope, got %d", len(initScopes))
		}
		if len(funcScopes) != 1 {
			t.Errorf("expected 1 function scope (Calculate), got %d", len(funcScopes))
		}
	})

	t.Run("backward compatibility - Lines contains function lines only", func(t *testing.T) {
		root := "../../examples/compat"

		wf := NewWorkflow(adapter.NewLocalSourceFSAdapter(), adapter.NewLocalGoFileAdapter(), adapter.NewLocalTestRunnerAdapter())
		sources, err := wf.GetSources(m.Path(root))
		if err != nil {
			t.Fatalf("GetSources error: %v", err)
		}

		if len(sources) == 0 {
			t.Fatalf("expected to find source")
		}

		compatPath := filepath.Join(root, "main.go")
		source, ok := findSourceFor(sources, compatPath)
		if !ok {
			t.Fatalf("expected to find source for %s", compatPath)
		}

		// Lines field should only contain function start lines (5 and 9)
		if len(source.Lines) != 2 {
			t.Errorf("expected 2 function lines, got %d: %v", len(source.Lines), source.Lines)
		}

		// Should NOT contain const line (3)
		if containsInt(source.Lines, 3) {
			t.Errorf("Lines should not contain const declaration line")
		}

		// Should contain function lines
		if !containsInt(source.Lines, 5) {
			t.Errorf("expected Lines to contain Calculate function line 5")
		}
		if !containsInt(source.Lines, 9) {
			t.Errorf("expected Lines to contain Validate function line 9")
		}
	})

	t.Run("excludes files with only type declarations", func(t *testing.T) {
		root := "../../examples/types"

		wf := NewWorkflow(adapter.NewLocalSourceFSAdapter(), adapter.NewLocalGoFileAdapter(), adapter.NewLocalTestRunnerAdapter())
		sources, err := wf.GetSources(m.Path(root))
		if err != nil {
			t.Fatalf("GetSources error: %v", err)
		}

		// Files with only type declarations should be excluded
		if len(sources) != 0 {
			t.Errorf("expected 0 sources for file with only types, got %d", len(sources))
		}
	})

	t.Run("example basic has functions", func(t *testing.T) {
		root := "../../examples/basic"

		wf := NewWorkflow(adapter.NewLocalSourceFSAdapter(), adapter.NewLocalGoFileAdapter(), adapter.NewLocalTestRunnerAdapter())
		sources, err := wf.GetSources(m.Path(root))
		if err != nil {
			t.Fatalf("GetSources error: %v", err)
		}

		if len(sources) == 0 {
			t.Fatalf("expected sources in basic example")
		}
	})

	t.Run("handles ./... pattern for recursive scanning", func(t *testing.T) {
		root := "../../examples/nested/..."

		wf := NewWorkflow(adapter.NewLocalSourceFSAdapter(), adapter.NewLocalGoFileAdapter(), adapter.NewLocalTestRunnerAdapter())
		sources, err := wf.GetSources(m.Path(root))
		if err != nil {
			t.Fatalf("GetSources error: %v", err)
		}

		// Should find child.go in sub/ directory
		childPath := filepath.Join("../../examples/nested/sub", "child.go")
		if _, ok := findSourceFor(sources, childPath); !ok {
			t.Errorf("expected to find nested source with ./... pattern")
		}
	})

	t.Run("non-recursive without ./... stops at directory level", func(t *testing.T) {
		root := "../../examples/nested"

		wf := NewWorkflow(adapter.NewLocalSourceFSAdapter(), adapter.NewLocalGoFileAdapter(), adapter.NewLocalTestRunnerAdapter())
		sources, err := wf.GetSources(m.Path(root))
		if err != nil {
			t.Fatalf("GetSources error: %v", err)
		}

		// Should NOT find child.go in sub/ without recursive pattern
		childPath := filepath.Join("../../examples/nested/sub", "child.go")
		if _, ok := findSourceFor(sources, childPath); ok {
			t.Errorf("should not find nested source without ./... pattern")
		}
	})

	t.Run("handles multiple paths in single call", func(t *testing.T) {
		path1 := "../../examples/constants"
		path2 := "../../examples/variables"

		wf := NewWorkflow(adapter.NewLocalSourceFSAdapter(), adapter.NewLocalGoFileAdapter(), adapter.NewLocalTestRunnerAdapter())
		sources, err := wf.GetSources(m.Path(path1), m.Path(path2))
		if err != nil {
			t.Fatalf("GetSources error: %v", err)
		}

		// Should find sources from both paths
		constPath := filepath.Join(path1, "main.go")
		varPath := filepath.Join(path2, "main.go")

		foundConst := false
		foundVar := false
		for _, s := range sources {
			if filepath.Clean(string(s.Origin)) == filepath.Clean(constPath) {
				foundConst = true
			}
			if filepath.Clean(string(s.Origin)) == filepath.Clean(varPath) {
				foundVar = true
			}
		}

		if !foundConst {
			t.Errorf("expected to find source from constants path")
		}
		if !foundVar {
			t.Errorf("expected to find source from variables path")
		}
	})

	t.Run("handles mix of recursive and non-recursive paths", func(t *testing.T) {
		recursivePath := "../../examples/nested/..."
		nonRecursivePath := "../../examples/constants"

		wf := NewWorkflow(adapter.NewLocalSourceFSAdapter(), adapter.NewLocalGoFileAdapter(), adapter.NewLocalTestRunnerAdapter())
		sources, err := wf.GetSources(m.Path(recursivePath), m.Path(nonRecursivePath))
		if err != nil {
			t.Fatalf("GetSources error: %v", err)
		}

		// Should find nested child.go (recursive)
		childPath := filepath.Join("../../examples/nested/sub", "child.go")
		foundChild := false

		// Should find constants/main.go (non-recursive)
		constPath := filepath.Join(nonRecursivePath, "main.go")
		foundConst := false

		for _, s := range sources {
			cleanOrigin := filepath.Clean(string(s.Origin))
			if cleanOrigin == filepath.Clean(childPath) {
				foundChild = true
			}
			if cleanOrigin == filepath.Clean(constPath) {
				foundConst = true
			}
		}

		if !foundChild {
			t.Errorf("expected to find nested source with recursive path")
		}
		if !foundConst {
			t.Errorf("expected to find constants source with non-recursive path")
		}
	})

	t.Run("./... pattern on single file directory works", func(t *testing.T) {
		root := "../../examples/constants/..."

		wf := NewWorkflow(adapter.NewLocalSourceFSAdapter(), adapter.NewLocalGoFileAdapter(), adapter.NewLocalTestRunnerAdapter())
		sources, err := wf.GetSources(m.Path(root))
		if err != nil {
			t.Fatalf("GetSources error: %v", err)
		}

		if len(sources) == 0 {
			t.Fatalf("expected to find sources even with ./... on single-level dir")
		}
	})

	t.Run("empty path list returns empty sources", func(t *testing.T) {
		wf := NewWorkflow(adapter.NewLocalSourceFSAdapter(), adapter.NewLocalGoFileAdapter(), adapter.NewLocalTestRunnerAdapter())
		sources, err := wf.GetSources()
		if err != nil {
			t.Fatalf("GetSources error: %v", err)
		}
		if len(sources) != 0 {
			t.Errorf("expected 0 sources for empty path list, got %d", len(sources))
		}
	})

	t.Run("multiple directories without recursion", func(t *testing.T) {
		dir1 := "../../examples/constants"
		dir2 := "../../examples/variables"
		dir3 := "../../examples/initfunc"

		wf := NewWorkflow(adapter.NewLocalSourceFSAdapter(), adapter.NewLocalGoFileAdapter(), adapter.NewLocalTestRunnerAdapter())
		sources, err := wf.GetSources(m.Path(dir1), m.Path(dir2), m.Path(dir3))
		if err != nil {
			t.Fatalf("GetSources error: %v", err)
		}

		// Should have sources from all three directories
		if len(sources) < 3 {
			t.Errorf("expected at least 3 sources from 3 directories, got %d", len(sources))
		}

		// Verify we got sources from each directory
		foundDirs := make(map[string]bool)
		for _, s := range sources {
			dir := filepath.Dir(string(s.Origin))
			foundDirs[dir] = true
		}

		expectedDirs := []string{
			filepath.Clean(dir1),
			filepath.Clean(dir2),
			filepath.Clean(dir3),
		}

		for _, expected := range expectedDirs {
			found := false
			for dir := range foundDirs {
				if filepath.Clean(dir) == expected {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected to find sources from directory %s", expected)
			}
		}
	})

	t.Run("multiple directories with ./... recursive pattern", func(t *testing.T) {
		dir1 := "../../examples/nested/..."
		dir2 := "../../examples/basic/..."

		wf := NewWorkflow(adapter.NewLocalSourceFSAdapter(), adapter.NewLocalGoFileAdapter(), adapter.NewLocalTestRunnerAdapter())
		sources, err := wf.GetSources(m.Path(dir1), m.Path(dir2))
		if err != nil {
			t.Fatalf("GetSources error: %v", err)
		}

		// Should find nested/sub/child.go
		nestedChild := filepath.Join("../../examples/nested/sub", "child.go")
		foundNested := false

		// Should find basic/main.go
		basicMain := filepath.Join("../../examples/basic", "main.go")
		foundBasic := false

		for _, s := range sources {
			cleanOrigin := filepath.Clean(string(s.Origin))
			if cleanOrigin == filepath.Clean(nestedChild) {
				foundNested = true
			}
			if cleanOrigin == filepath.Clean(basicMain) {
				foundBasic = true
			}
		}

		if !foundNested {
			t.Errorf("expected to find nested/sub/child.go with recursive pattern")
		}
		if !foundBasic {
			t.Errorf("expected to find basic/main.go with recursive pattern")
		}
	})

	t.Run("three directories with mixed recursive and non-recursive", func(t *testing.T) {
		recursive1 := "../../examples/nested/..."
		nonRecursive := "../../examples/constants"
		recursive2 := "../../examples/basic/..."

		wf := NewWorkflow(adapter.NewLocalSourceFSAdapter(), adapter.NewLocalGoFileAdapter(), adapter.NewLocalTestRunnerAdapter())
		sources, err := wf.GetSources(m.Path(recursive1), m.Path(nonRecursive), m.Path(recursive2))
		if err != nil {
			t.Fatalf("GetSources error: %v", err)
		}

		// Track what we found
		nestedChild := filepath.Join("../../examples/nested/sub", "child.go")
		constMain := filepath.Join("../../examples/constants", "main.go")
		basicMain := filepath.Join("../../examples/basic", "main.go")

		foundNested := false
		foundConst := false
		foundBasic := false

		for _, s := range sources {
			cleanOrigin := filepath.Clean(string(s.Origin))
			if cleanOrigin == filepath.Clean(nestedChild) {
				foundNested = true
			}
			if cleanOrigin == filepath.Clean(constMain) {
				foundConst = true
			}
			if cleanOrigin == filepath.Clean(basicMain) {
				foundBasic = true
			}
		}

		if !foundNested {
			t.Errorf("expected to find nested/sub/child.go from first recursive path")
		}
		if !foundConst {
			t.Errorf("expected to find constants/main.go from non-recursive path")
		}
		if !foundBasic {
			t.Errorf("expected to find basic/main.go from second recursive path")
		}
	})

	t.Run("multiple paths with duplicates are deduplicated", func(t *testing.T) {
		dir := "../../examples/constants"

		wf := NewWorkflow(adapter.NewLocalSourceFSAdapter(), adapter.NewLocalGoFileAdapter(), adapter.NewLocalTestRunnerAdapter())
		// Pass same directory twice
		sources, err := wf.GetSources(m.Path(dir), m.Path(dir))
		if err != nil {
			t.Fatalf("GetSources error: %v", err)
		}

		// Count occurrences of each file
		fileCounts := make(map[string]int)
		for _, s := range sources {
			fileCounts[string(s.Origin)]++
		}

		// Check for duplicates
		for file, count := range fileCounts {
			if count > 1 {
				t.Errorf("file %s appears %d times, expected 1 (no deduplication happening)", file, count)
			}
		}
	})
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func findLinesFor(sources []m.Source, origin string) ([]int, bool) {
	for _, s := range sources {
		if filepath.Clean(string(s.Origin)) == filepath.Clean(origin) {
			return s.Lines, true
		}
	}
	return nil, false
}

func containsInt(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

func findSourceFor(sources []m.Source, origin string) (m.Source, bool) {
	for _, s := range sources {
		if filepath.Clean(string(s.Origin)) == filepath.Clean(origin) {
			return s, true
		}
	}
	return m.Source{}, false
}

func filterScopesByType(scopes []m.CodeScope, scopeType m.ScopeType) []m.CodeScope {
	var result []m.CodeScope
	for _, scope := range scopes {
		if scope.Type == scopeType {
			result = append(result, scope)
		}
	}
	return result
}

func hasScopeWithName(scopes []m.CodeScope, name string) bool {
	for _, scope := range scopes {
		if scope.Name == name {
			return true
		}
	}
	return false
}

func copyExampleDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyExampleDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			content, err := os.ReadFile(srcPath)
			if err != nil {
				return err
			}
			if err := os.WriteFile(dstPath, content, 0o644); err != nil {
				return err
			}
		}
	}

	return nil
}

func TestTestMutation(t *testing.T) {
	t.Run("kills mutation when test fails", func(t *testing.T) {
		root := t.TempDir()

		// Copy example project with its test file
		srcDir := "../../examples/basic"
		if err := copyExampleDir(srcDir, root); err != nil {
			t.Fatalf("failed to copy example: %v", err)
		}

		wf := NewWorkflow(adapter.NewLocalSourceFSAdapter(), adapter.NewLocalGoFileAdapter(), adapter.NewLocalTestRunnerAdapter())
		sources, err := wf.GetSources(m.Path(root))
		if err != nil {
			t.Fatalf("GetSources error: %v", err)
		}
		if len(sources) == 0 {
			t.Fatalf("expected at least one source")
		}

		mainSource := sources[0]
		testGo := filepath.Join(root, "main_test.go")
		mainSource.Test = m.Path(testGo)

		mutations, err := wf.GenerateMutations(mainSource, m.MutationArithmetic)
		if err != nil {
			t.Fatalf("GenerateMutations error: %v", err)
		}
		if len(mutations) == 0 {
			t.Fatalf("expected at least one mutation")
		}

		mutation := mutations[0]
		report, err := wf.TestMutation(mainSource, mutation)
		if err != nil {
			t.Fatalf("TestMutation error: %v", err)
		}

		if !report.Killed {
			t.Errorf("expected mutation to be killed, but it survived")
		}

		if report.MutationID != mutation.ID {
			t.Errorf("report.MutationID = %s, want %s", report.MutationID, mutation.ID)
		}
	})

	t.Run("subfolder source - mutation killed", func(t *testing.T) {
		root := t.TempDir()

		// Create a small module with nested package
		writeFile(t, filepath.Join(root, "go.mod"), "module example.com/submut\n\ngo 1.22\n")

		subdir := filepath.Join(root, "pkg")
		if err := os.MkdirAll(subdir, 0o755); err != nil {
			t.Fatalf("mkdir subdir: %v", err)
		}

		// Source file inside subfolder with arithmetic to mutate
		writeFile(t, filepath.Join(subdir, "calc.go"), `package pkg

func Calculate(a, b int) int {
    return a + b
}
`)

		// Matching test in the same subfolder
		writeFile(t, filepath.Join(subdir, "calc_test.go"), `package pkg

import "testing"

func TestCalculate(t *testing.T) {
    if got := Calculate(2, 1); got != 3 {
        t.Fatalf("want 3, got %d", got)
    }
}
`)

		wf := NewWorkflow(adapter.NewLocalSourceFSAdapter(), adapter.NewLocalGoFileAdapter(), adapter.NewLocalTestRunnerAdapter())
		sources, err := wf.GetSources(m.Path(filepath.Join(root, "...")))
		if err != nil {
			t.Fatalf("GetSources error: %v", err)
		}
		if len(sources) == 0 {
			t.Fatalf("expected at least one source")
		}

		// Find calc.go inside subfolder
		var calcSrc m.Source
		found := false
		for _, s := range sources {
			if filepath.Base(string(s.Origin)) == "calc.go" {
				calcSrc = s
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("calc.go not found in sources")
		}

		muts, err := wf.GenerateMutations(calcSrc, m.MutationArithmetic)
		if err != nil {
			t.Fatalf("GenerateMutations error: %v", err)
		}
		if len(muts) == 0 {
			t.Fatalf("expected at least one mutation in calc.go")
		}

		rep, err := wf.TestMutation(calcSrc, muts[0])
		if err != nil {
			t.Fatalf("TestMutation error: %v", err)
		}
		if !rep.Killed {
			t.Errorf("expected mutation to be killed for subfolder source")
		}
	})

	t.Run("mutation survives when no test file", func(t *testing.T) {
		root := t.TempDir()

		// Copy example without test file
		srcDir := "../../examples/basic"
		if err := copyExampleDir(srcDir, root); err != nil {
			t.Fatalf("failed to copy example: %v", err)
		}

		// Remove test file to simulate no tests
		os.Remove(filepath.Join(root, "main_test.go"))

		wf := NewWorkflow(adapter.NewLocalSourceFSAdapter(), adapter.NewLocalGoFileAdapter(), adapter.NewLocalTestRunnerAdapter())
		sources, err := wf.GetSources(m.Path(root))
		if err != nil {
			t.Fatalf("GetSources error: %v", err)
		}
		if len(sources) == 0 {
			t.Fatalf("expected at least one source")
		}

		mainSource := sources[0]
		// No test file set (Test field is empty)

		mutations, err := wf.GenerateMutations(mainSource, m.MutationArithmetic)
		if err != nil {
			t.Fatalf("GenerateMutations error: %v", err)
		}
		if len(mutations) == 0 {
			t.Fatalf("expected at least one mutation")
		}

		mutation := mutations[0]

		report, err := wf.TestMutation(mainSource, mutation)
		if err != nil {
			t.Fatalf("TestMutation error: %v", err)
		}

		// Without tests, mutation survives
		if report.Killed {
			t.Errorf("expected mutation to survive (no tests), but it was killed")
		}
	})

	t.Run("returns error for invalid mutation", func(t *testing.T) {
		root := t.TempDir()

		// Copy example (test file not needed for this test)
		srcDir := "../../examples/basic"
		if err := copyExampleDir(srcDir, root); err != nil {
			t.Fatalf("failed to copy example: %v", err)
		}

		wf := NewWorkflow(adapter.NewLocalSourceFSAdapter(), adapter.NewLocalGoFileAdapter(), adapter.NewLocalTestRunnerAdapter())
		sources, err := wf.GetSources(m.Path(root))
		if err != nil {
			t.Fatalf("GetSources error: %v", err)
		}

		mainSource := sources[0]

		// Create an invalid mutation (non-existent line)
		invalidMutation := m.Mutation{
			ID:         "INVALID_1",
			Type:       m.MutationArithmetic,
			SourceFile: mainSource.Origin,
			OriginalOp: token.ADD,
			MutatedOp:  token.SUB,
			Line:       9999, // line doesn't exist
			Column:     1,
		}

		_, err = wf.TestMutation(mainSource, invalidMutation)
		if err == nil {
			t.Errorf("expected error for invalid mutation, got nil")
		}
	})

	t.Run("automatic test file detection with TestMutation", func(t *testing.T) {
		root := t.TempDir()

		// Copy example with test file
		srcDir := "../../examples/basic"
		if err := copyExampleDir(srcDir, root); err != nil {
			t.Fatalf("failed to copy example: %v", err)
		}

		wf := NewWorkflow(adapter.NewLocalSourceFSAdapter(), adapter.NewLocalGoFileAdapter(), adapter.NewLocalTestRunnerAdapter())

		// Get sources - should automatically detect test file
		sources, err := wf.GetSources(m.Path(root))
		if err != nil {
			t.Fatalf("GetSources error: %v", err)
		}
		if len(sources) == 0 {
			t.Fatalf("expected at least one source")
		}

		mainSource := sources[0]

		// Verify test file was auto-detected
		if mainSource.Test == "" {
			t.Fatalf("expected test file to be auto-detected, got empty")
		}

		// Generate and test mutation WITHOUT manually setting Test field
		mutations, err := wf.GenerateMutations(mainSource, m.MutationArithmetic)
		if err != nil {
			t.Fatalf("GenerateMutations error: %v", err)
		}
		if len(mutations) == 0 {
			t.Fatalf("expected at least one mutation")
		}

		// Test mutation with auto-detected test file
		report, err := wf.TestMutation(mainSource, mutations[0])
		if err != nil {
			t.Fatalf("TestMutation error: %v", err)
		}

		// Mutation should be killed by auto-detected test
		if !report.Killed {
			t.Errorf("expected mutation to be killed by auto-detected test")
		}
	})

	t.Run("example basic - mutation with correct test", func(t *testing.T) {
		root := t.TempDir()

		// Copy basic example with test file
		srcDir := "../../examples/basic"
		if err := copyExampleDir(srcDir, root); err != nil {
			t.Fatalf("failed to copy example: %v", err)
		}

		wf := NewWorkflow(adapter.NewLocalSourceFSAdapter(), adapter.NewLocalGoFileAdapter(), adapter.NewLocalTestRunnerAdapter())
		sources, err := wf.GetSources(m.Path(root))
		if err != nil {
			t.Fatalf("GetSources error: %v", err)
		}

		var mainSource m.Source
		for _, s := range sources {
			if filepath.Base(string(s.Origin)) == "main.go" {
				mainSource = s
				break
			}
		}

		mutations, err := wf.GenerateMutations(mainSource, m.MutationArithmetic)
		if err != nil {
			t.Fatalf("GenerateMutations error: %v", err)
		}
		if len(mutations) == 0 {
			t.Fatalf("expected mutations for basic example")
		}

		// Test first mutation
		report, err := wf.TestMutation(mainSource, mutations[0])
		if err != nil {
			t.Fatalf("TestMutation error: %v", err)
		}

		if !report.Killed {
			t.Errorf("expected mutation to be killed by correct test")
		}
	})

	t.Run("example scopes - mutation in Calculate function", func(t *testing.T) {
		root := t.TempDir()

		// Copy scopes example with test file
		srcDir := "../../examples/scopes"
		if err := copyExampleDir(srcDir, root); err != nil {
			t.Fatalf("failed to copy example: %v", err)
		}

		wf := NewWorkflow(adapter.NewLocalSourceFSAdapter(), adapter.NewLocalGoFileAdapter(), adapter.NewLocalTestRunnerAdapter())
		sources, err := wf.GetSources(m.Path(root))
		if err != nil {
			t.Fatalf("GetSources error: %v", err)
		}

		mainSource := sources[0]

		mutations, err := wf.GenerateMutations(mainSource, m.MutationArithmetic)
		if err != nil {
			t.Fatalf("GenerateMutations error: %v", err)
		}
		if len(mutations) == 0 {
			t.Fatalf("expected mutations in Calculate function")
		}

		// Find mutation in function scope (Calculate function has arithmetic)
		var funcMutation m.Mutation
		foundFunc := false
		for _, mut := range mutations {
			if mut.ScopeType == m.ScopeFunction {
				funcMutation = mut
				foundFunc = true
				break
			}
		}

		if !foundFunc {
			t.Fatalf("expected to find mutation in function scope")
		}

		report, err := wf.TestMutation(mainSource, funcMutation)
		if err != nil {
			t.Fatalf("TestMutation error: %v", err)
		}

		if !report.Killed {
			t.Errorf("expected mutation in Calculate to be killed")
		}
	})

	t.Run("example mixed - multiple functions", func(t *testing.T) {
		root := t.TempDir()

		// Copy mixed example with test file
		srcDir := "../../examples/mixed"
		if err := copyExampleDir(srcDir, root); err != nil {
			t.Fatalf("failed to copy example: %v", err)
		}

		wf := NewWorkflow(adapter.NewLocalSourceFSAdapter(), adapter.NewLocalGoFileAdapter(), adapter.NewLocalTestRunnerAdapter())
		sources, err := wf.GetSources(m.Path(root))
		if err != nil {
			t.Fatalf("GetSources error: %v", err)
		}

		mainSource := sources[0]

		mutations, err := wf.GenerateMutations(mainSource, m.MutationArithmetic)
		if err != nil {
			t.Fatalf("GenerateMutations error: %v", err)
		}

		// Should have mutations for *
		if len(mutations) < 1 {
			t.Fatalf("expected at least 1 mutation, got %d", len(mutations))
		}

		// Test all mutations
		killedCount := 0
		for _, mutation := range mutations {
			report, err := wf.TestMutation(mainSource, mutation)
			if err != nil {
				t.Errorf("TestMutation error for %s: %v", mutation.ID, err)
				continue
			}
			if report.Killed {
				killedCount++
			}
		}

		if killedCount == 0 {
			t.Errorf("expected at least some mutations to be killed")
		}
	})

	t.Run("complex example - multiple operators in one line", func(t *testing.T) {
		root := t.TempDir()

		// Use basic example which has inline arithmetic with test file
		srcDir := "../../examples/basic"
		if err := copyExampleDir(srcDir, root); err != nil {
			t.Fatalf("failed to copy example: %v", err)
		}

		wf := NewWorkflow(adapter.NewLocalSourceFSAdapter(), adapter.NewLocalGoFileAdapter(), adapter.NewLocalTestRunnerAdapter())
		sources, err := wf.GetSources(m.Path(root))
		if err != nil {
			t.Fatalf("GetSources error: %v", err)
		}

		mainSource := sources[0]

		mutations, err := wf.GenerateMutations(mainSource, m.MutationArithmetic)
		if err != nil {
			t.Fatalf("GenerateMutations error: %v", err)
		}

		// Should have mutations
		if len(mutations) < 1 {
			t.Fatalf("expected at least 1 mutation, got %d", len(mutations))
		}

		// At least one mutation should be killed
		anyKilled := false
		for _, mutation := range mutations {
			report, err := wf.TestMutation(mainSource, mutation)
			if err != nil {
				t.Errorf("TestMutation error: %v", err)
				continue
			}
			if report.Killed {
				anyKilled = true
				break
			}
		}

		if !anyKilled {
			t.Errorf("expected at least one mutation to be killed")
		}
	})
}
