package mutagens

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	m "github.com/mouse-blink/gooze/internal/model"
)

func TestGenerateComparisonMutations(t *testing.T) {
	tests := []struct {
		name          string
		code          string
		expectedCount int
		expectedType  m.MutationType
	}{
		{
			name:          "less than operator",
			code:          "package main\nfunc test() { x := 5 < 10 }",
			expectedCount: 5, // <, >, <=, >=, ==, != minus original <
			expectedType:  m.MutationComparison,
		},
		{
			name:          "greater than operator",
			code:          "package main\nfunc test() { x := 5 > 10 }",
			expectedCount: 5,
			expectedType:  m.MutationComparison,
		},
		{
			name:          "less than or equal operator",
			code:          "package main\nfunc test() { x := 5 <= 10 }",
			expectedCount: 5,
			expectedType:  m.MutationComparison,
		},
		{
			name:          "greater than or equal operator",
			code:          "package main\nfunc test() { x := 5 >= 10 }",
			expectedCount: 5,
			expectedType:  m.MutationComparison,
		},
		{
			name:          "equal operator",
			code:          "package main\nfunc test() { x := 5 == 10 }",
			expectedCount: 5,
			expectedType:  m.MutationComparison,
		},
		{
			name:          "not equal operator",
			code:          "package main\nfunc test() { x := 5 != 10 }",
			expectedCount: 5,
			expectedType:  m.MutationComparison,
		},
		{
			name:          "non-comparison operator",
			code:          "package main\nfunc test() { x := 5 + 10 }",
			expectedCount: 0,
			expectedType:  m.MutationComparison,
		},
		{
			name:          "multiple comparison operators",
			code:          "package main\nfunc test() { x := 5 < 10 && 20 > 15 }",
			expectedCount: 10, // 5 mutations for each operator
			expectedType:  m.MutationComparison,
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

			// Traverse AST and collect mutations
			ast.Inspect(file, func(n ast.Node) bool {
				mutations = append(mutations, GenerateComparisonMutations(n, fset, content, source)...)
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

func TestIsComparisonOp(t *testing.T) {
	tests := []struct {
		name     string
		op       token.Token
		expected bool
	}{
		{"less than", token.LSS, true},
		{"greater than", token.GTR, true},
		{"less than or equal", token.LEQ, true},
		{"greater than or equal", token.GEQ, true},
		{"equal", token.EQL, true},
		{"not equal", token.NEQ, true},
		{"addition", token.ADD, false},
		{"subtraction", token.SUB, false},
		{"logical and", token.LAND, false},
		{"logical or", token.LOR, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isComparisonOp(tt.op)
			if result != tt.expected {
				t.Errorf("isComparisonOp(%v) = %v, expected %v", tt.op, result, tt.expected)
			}
		})
	}
}

func TestGetComparisonAlternatives(t *testing.T) {
	tests := []struct {
		name             string
		original         token.Token
		expectedCount    int
		shouldNotInclude token.Token
	}{
		{"less than", token.LSS, 5, token.LSS},
		{"greater than", token.GTR, 5, token.GTR},
		{"less than or equal", token.LEQ, 5, token.LEQ},
		{"greater than or equal", token.GEQ, 5, token.GEQ},
		{"equal", token.EQL, 5, token.EQL},
		{"not equal", token.NEQ, 5, token.NEQ},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			alternatives := getComparisonAlternatives(tt.original)

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
