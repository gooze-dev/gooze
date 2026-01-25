package mutagens

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	m "github.com/mouse-blink/gooze/internal/model"
)

func TestGenerateUnaryMutations(t *testing.T) {
	tests := []struct {
		name          string
		code          string
		expectedCount int
	}{
		{
			name:          "negation operator",
			code:          "package main\nfunc f() int { return -5 }",
			expectedCount: 2, // -5 -> +5, -5 -> 5 (removal)
		},
		{
			name:          "positive operator",
			code:          "package main\nfunc f() int { return +5 }",
			expectedCount: 2, // +5 -> -5, +5 -> 5 (removal)
		},
		{
			name:          "logical NOT operator",
			code:          "package main\nfunc f() bool { return !true }",
			expectedCount: 1, // !true -> true (removal only)
		},
		{
			name:          "bitwise NOT operator",
			code:          "package main\nfunc f() int { return ^5 }",
			expectedCount: 1, // ^5 -> 5 (removal only)
		},
		{
			name:          "no unary operators",
			code:          "package main\nfunc f() int { return 5 }",
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "test.go", tt.code, parser.AllErrors)
			if err != nil {
				t.Fatalf("failed to parse code: %v", err)
			}

			source := m.Source{
				Origin: &m.File{
					FullPath: "test.go",
				},
			}

			var mutations []m.Mutation

			ast.Inspect(file, func(n ast.Node) bool {
				mutations = append(mutations, GenerateUnaryMutations(n, fset, []byte(tt.code), source)...)
				return true
			})

			if len(mutations) != tt.expectedCount {
				t.Errorf("expected %d mutations, got %d", tt.expectedCount, len(mutations))
			}

			// Verify mutation type
			for _, mut := range mutations {
				if mut.Type != m.MutationUnary {
					t.Errorf("expected mutation type %v, got %v", m.MutationUnary, mut.Type)
				}
			}
		})
	}
}

func TestIsUnaryOp(t *testing.T) {
	tests := []struct {
		name     string
		op       token.Token
		expected bool
	}{
		{"SUB is unary", token.SUB, true},
		{"ADD is unary", token.ADD, true},
		{"NOT is unary", token.NOT, true},
		{"XOR is unary", token.XOR, true},
		{"MUL is not unary", token.MUL, false},
		{"AND is not unary", token.AND, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isUnaryOp(tt.op)
			if result != tt.expected {
				t.Errorf("isUnaryOp(%v) = %v, want %v", tt.op, result, tt.expected)
			}
		})
	}
}

func TestGetUnaryAlternatives(t *testing.T) {
	tests := []struct {
		name          string
		op            token.Token
		expectedCount int
	}{
		{"SUB has one alternative", token.SUB, 1},
		{"ADD has one alternative", token.ADD, 1},
		{"NOT has no alternatives", token.NOT, 0},
		{"XOR has no alternatives", token.XOR, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			alternatives := getUnaryAlternatives(tt.op)
			if len(alternatives) != tt.expectedCount {
				t.Errorf("getUnaryAlternatives(%v) returned %d alternatives, want %d", tt.op, len(alternatives), tt.expectedCount)
			}
		})
	}
}
