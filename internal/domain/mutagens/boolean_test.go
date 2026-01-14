package mutagens

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"

	m "github.com/mouse-blink/gooze/internal/model"
)

func TestProcessBooleanMutations(t *testing.T) {
	t.Run("generates mutation for true literal", func(t *testing.T) {
		booleanPath := filepath.Join("..", "..", "..", "examples", "boolean", "main.go")

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, booleanPath, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("failed to parse source %s: %v", booleanPath, err)
		}

		source := m.Source{
			Origin: m.Path(booleanPath),
			Scopes: []m.CodeScope{
				// Treat the whole file as a single function scope for testing.
				{StartLine: 1, EndLine: 100, Type: m.ScopeFunction},
			},
		}

		var mutations []m.Mutation
		mutationID := 0

		ast.Inspect(file, func(n ast.Node) bool {
			mutations = append(mutations, ProcessBooleanMutations(n, fset, source, &mutationID)...)
			return true
		})

		if len(mutations) == 0 {
			t.Fatalf("expected at least 1 mutation, got 0")
		}

		found := false
		for _, mut := range mutations {
			if mut.Type != m.MutationBoolean {
				t.Errorf("expected boolean mutation, got %v", mut.Type)
			}
			if mut.OriginalText == "true" && mut.MutatedText == "false" {
				found = true
				if mut.ScopeType != m.ScopeFunction {
					t.Errorf("expected function scope, got %v", mut.ScopeType)
				}
			}
		}

		if !found {
			t.Errorf("expected at least one true -> false mutation")
		}
	})

	t.Run("generates mutation for false literal", func(t *testing.T) {
		booleanPath := filepath.Join("..", "..", "..", "examples", "boolean", "main.go")

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, booleanPath, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("failed to parse source %s: %v", booleanPath, err)
		}

		source := m.Source{
			Origin: m.Path(booleanPath),
			Scopes: []m.CodeScope{
				// Treat the whole file as a single function scope for testing.
				{StartLine: 1, EndLine: 100, Type: m.ScopeFunction},
			},
		}

		var mutations []m.Mutation
		mutationID := 0

		ast.Inspect(file, func(n ast.Node) bool {
			mutations = append(mutations, ProcessBooleanMutations(n, fset, source, &mutationID)...)
			return true
		})

		if len(mutations) == 0 {
			t.Fatalf("expected at least 1 mutation, got 0")
		}

		found := false
		for _, mut := range mutations {
			if mut.Type != m.MutationBoolean {
				t.Errorf("expected boolean mutation, got %v", mut.Type)
			}
			if mut.OriginalText == "false" && mut.MutatedText == "true" {
				found = true
			}
		}

		if !found {
			t.Errorf("expected at least one false -> true mutation")
		}
	})

	t.Run("returns empty slice for non-boolean literals", func(t *testing.T) {
		emptyPath := filepath.Join("..", "..", "..", "examples", "empty", "main.go")

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, emptyPath, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("failed to parse source %s: %v", emptyPath, err)
		}

		source := m.Source{
			Origin: m.Path(emptyPath),
			Scopes: []m.CodeScope{
				// Treat the whole file as a single function scope for testing.
				{StartLine: 1, EndLine: 100, Type: m.ScopeFunction},
			},
		}

		var mutations []m.Mutation
		mutationID := 0

		ast.Inspect(file, func(n ast.Node) bool {
			mutations = append(mutations, ProcessBooleanMutations(n, fset, source, &mutationID)...)
			return true
		})

		if len(mutations) != 0 {
			t.Errorf("expected no mutations for non-boolean literals, got %d", len(mutations))
		}
	})
}

func TestIsBooleanLiteral(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"true literal", "true", true},
		{"false literal", "false", true},
		{"variable name", "someVar", false},
		{"other string", "hello", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isBooleanLiteral(tt.input)
			if result != tt.expected {
				t.Errorf("isBooleanLiteral(%q) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFlipBoolean(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"true", "false"},
		{"false", "true"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := flipBoolean(tt.input)
			if result != tt.expected {
				t.Errorf("flipBoolean(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}
