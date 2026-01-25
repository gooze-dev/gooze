package mutagens

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	m "github.com/mouse-blink/gooze/internal/model"
)

func TestGenerateLogicalMutations(t *testing.T) {
	tests := []struct {
		name          string
		code          string
		expectedCount int
		expectedType  m.MutationType
	}{
		{
			name:          "logical AND operator",
			code:          "package main\nfunc test() { x := true && false }",
			expectedCount: 1,
			expectedType:  m.MutationLogical,
		},
		{
			name:          "logical OR operator",
			code:          "package main\nfunc test() { x := true || false }",
			expectedCount: 1,
			expectedType:  m.MutationLogical,
		},
		{
			name:          "multiple logical operators",
			code:          "package main\nfunc test() { x := true && false || true }",
			expectedCount: 2,
			expectedType:  m.MutationLogical,
		},
		{
			name:          "logical with comparison",
			code:          "package main\nfunc test() { x := (5 > 3) && (10 < 20) }",
			expectedCount: 1,
			expectedType:  m.MutationLogical,
		},
		{
			name:          "non-logical operator",
			code:          "package main\nfunc test() { x := 5 + 10 }",
			expectedCount: 0,
			expectedType:  m.MutationLogical,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "test.go", tt.code, parser.AllErrors)
			if err != nil {
				t.Fatalf("Failed to parse code: %v", err)
			}

			content := []byte(tt.code)
			source := m.Source{
				Origin: &m.File{FullPath: "test.go"},
			}
			mutations := []m.Mutation{}

			ast.Inspect(file, func(n ast.Node) bool {
				mutations = append(mutations, GenerateLogicalMutations(n, fset, content, source)...)
				return true
			})

			if len(mutations) != tt.expectedCount {
				t.Errorf("Expected %d mutations, got %d", tt.expectedCount, len(mutations))
			}

			for _, mut := range mutations {
				if mut.Type != tt.expectedType {
					t.Errorf("Expected mutation type %v, got %v", tt.expectedType, mut.Type)
				}
			}
		})
	}
}

func TestIsLogicalOp(t *testing.T) {
	tests := []struct {
		name     string
		op       token.Token
		expected bool
	}{
		{"logical AND", token.LAND, true},
		{"logical OR", token.LOR, true},
		{"bitwise AND", token.AND, false},
		{"bitwise OR", token.OR, false},
		{"addition", token.ADD, false},
		{"equal", token.EQL, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isLogicalOp(tt.op)
			if result != tt.expected {
				t.Errorf("isLogicalOp(%v) = %v, expected %v", tt.op, result, tt.expected)
			}
		})
	}
}

func TestGetLogicalAlternatives(t *testing.T) {
	tests := []struct {
		name             string
		original         token.Token
		expectedCount    int
		shouldNotInclude token.Token
	}{
		{"logical AND", token.LAND, 1, token.LAND},
		{"logical OR", token.LOR, 1, token.LOR},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			alternatives := getLogicalAlternatives(tt.original)

			if len(alternatives) != tt.expectedCount {
				t.Errorf("Expected %d alternatives, got %d", tt.expectedCount, len(alternatives))
			}

			for _, alt := range alternatives {
				if alt == tt.shouldNotInclude {
					t.Errorf("Alternatives should not include original operator %v", tt.original)
				}
			}
		})
	}
}
