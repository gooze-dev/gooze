package domain

import (
	"os"
	"path/filepath"
	"testing"

	m "github.com/mouse-blink/gooze/internal/model"
)

func TestGetSources(t *testing.T) {
	t.Run("detects functions in files", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, filepath.Join(root, "main.go"), "package main\n\nfunc main() {}\n")
		writeFile(t, filepath.Join(root, "type.go"), "package main\n\ntype A struct { x int }\n")

		wf := NewWorkflow()
		sources, err := wf.GetSources(m.Path(root))
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
		// `main` starts at line 3 in our fixture.
		if !containsInt(lines, 3) {
			t.Errorf("expected function at line 3 in %s, got %v", mainPath, lines)
		}
		// Ensure type.go (no functions) is not included in results.
		if _, present := findLinesFor(sources, filepath.Join(root, "type.go")); present {
			t.Errorf("did not expect type.go to be reported (contains no functions)")
		}
	})

	t.Run("excludes files without functions", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, filepath.Join(root, "type.go"), "package main\n\ntype A struct { x int }\n")

		wf := NewWorkflow()
		sources, err := wf.GetSources(m.Path(root))
		if err != nil {
			t.Fatalf("GetSources error: %v", err)
		}
		if len(sources) != 0 {
			t.Fatalf("expected 0 sources, got %d", len(sources))
		}
	})

	t.Run("walks nested directories", func(t *testing.T) {
		root := t.TempDir()
		nested := filepath.Join(root, "sub")
		if err := os.MkdirAll(nested, 0o755); err != nil {
			t.Fatalf("mkdir nested: %v", err)
		}
		writeFile(t, filepath.Join(nested, "child.go"), "package sub\n\nfunc Hello() {}\n")

		wf := NewWorkflow()
		sources, err := wf.GetSources(m.Path(root))
		if err != nil {
			t.Fatalf("GetSources error: %v", err)
		}
		childPath := filepath.Join(nested, "child.go")
		if _, ok := findLinesFor(sources, childPath); !ok {
			t.Fatalf("expected to find nested source %s", childPath)
		}
	})

	t.Run("nonexistent root returns error", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "no_such_dir")
		wf := NewWorkflow()
		_, err := wf.GetSources(m.Path(root))
		if err == nil {
			t.Fatalf("expected error for nonexistent root")
		}
	})

	t.Run("empty directory returns no sources", func(t *testing.T) {
		root := t.TempDir()
		wf := NewWorkflow()
		sources, err := wf.GetSources(m.Path(root))
		if err != nil {
			t.Fatalf("GetSources error: %v", err)
		}
		if len(sources) != 0 {
			t.Fatalf("expected 0 sources in empty dir, got %d", len(sources))
		}
	})

	t.Run("invalid Go file yields error", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, filepath.Join(root, "broken.go"), "package main\n\nfunc Broken(\n")
		wf := NewWorkflow()
		_, err := wf.GetSources(m.Path(root))
		if err == nil {
			t.Fatalf("expected error for invalid Go files under %s", root)
		}
	})

	t.Run("detects global constants with proper scope", func(t *testing.T) {
		root := "../../examples/constants"

		wf := NewWorkflow()
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

		wf := NewWorkflow()
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

		wf := NewWorkflow()
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

		wf := NewWorkflow()
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

		wf := NewWorkflow()
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

		wf := NewWorkflow()
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

		wf := NewWorkflow()
		sources, err := wf.GetSources(m.Path(root))
		if err != nil {
			t.Fatalf("GetSources error: %v", err)
		}

		if len(sources) == 0 {
			t.Fatalf("expected sources in basic example")
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
