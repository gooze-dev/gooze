package mutagens

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	m "github.com/mouse-blink/gooze/internal/model"
)

func TestGenerateBooleanMutations(t *testing.T) {
	booleanPath := filepath.Join("..", "..", "..", "examples", "boolean", "main.go")
	content, err := os.ReadFile(booleanPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", booleanPath, err)
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, booleanPath, content, parser.ParseComments)
	if err != nil {
		t.Fatalf("failed to parse source %s: %v", booleanPath, err)
	}

	source := m.Source{Origin: &m.File{FullPath: m.Path(booleanPath)}}
	var mutations []m.Mutation

	ast.Inspect(file, func(n ast.Node) bool {
		mutations = append(mutations, GenerateBooleanMutations(n, fset, content, source)...)
		return true
	})

	if len(mutations) != 4 {
		t.Fatalf("expected 4 mutations, got %d", len(mutations))
	}

	expectedFragments := map[string]bool{
		"isValid := false":          false,
		"isComplete := true":        false,
		"checkStatus(false, false)": false,
		"checkStatus(true, true)":   false,
	}

	for i, mutation := range mutations {
		if len(mutation.ID) == 0 {
			t.Fatalf("expected non-empty mutation ID for mutation %d", i)
		}
		if mutation.Type != m.MutationBoolean {
			t.Fatalf("expected boolean mutation, got %v", mutation.Type)
		}
		if mutation.Source.Origin == nil || mutation.Source.Origin.FullPath != m.Path(booleanPath) {
			t.Fatalf("unexpected source origin: %+v", mutation.Source.Origin)
		}
		if bytes.Equal(mutation.MutatedCode, content) {
			t.Fatalf("expected mutated code to differ from original")
		}

		// Test that DiffCode is generated
		if len(mutation.DiffCode) == 0 {
			t.Fatalf("expected DiffCode to be generated for mutation %d", i)
		}
		diffStr := string(mutation.DiffCode)
		if !strings.Contains(diffStr, "--- original") || !strings.Contains(diffStr, "+++ mutated") {
			t.Fatalf("expected valid diff format, got: %s", diffStr)
		}

		mutated := string(mutation.MutatedCode)
		for fragment := range expectedFragments {
			if strings.Contains(mutated, fragment) {
				expectedFragments[fragment] = true
			}
		}
	}

	for fragment, found := range expectedFragments {
		if !found {
			t.Errorf("expected mutated code to contain %q", fragment)
		}
	}
}

func TestIsBooleanLiteral(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"true", true},
		{"false", true},
		{"True", false},
		{"FALSE", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if result := isBooleanLiteralV2(tt.name); result != tt.expected {
				t.Fatalf("isBooleanLiteralV2(%q) = %v, expected %v", tt.name, result, tt.expected)
			}
		})
	}
}
