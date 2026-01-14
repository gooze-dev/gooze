package mutagens

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"

	m "github.com/mouse-blink/gooze/internal/model"
)

func TestProcessArithmeticMutations(t *testing.T) {
	t.Run("generates mutations for binary expression with ADD operator", func(t *testing.T) {
		basicPath := filepath.Join("..", "..", "..", "examples", "basic", "main.go")

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, basicPath, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("failed to parse source %s: %v", basicPath, err)
		}

		source := m.Source{
			Origin: m.Path(basicPath),
			Scopes: []m.CodeScope{
				// Treat the whole file as a single function scope for testing.
				{StartLine: 1, EndLine: 100, Type: m.ScopeFunction},
			},
		}

		var mutations []m.Mutation
		mutationID := 0

		ast.Inspect(file, func(n ast.Node) bool {
			mutations = append(mutations, ProcessArithmeticMutations(n, fset, source, &mutationID)...)
			return true
		})

		// Should generate 4 mutations: + → -, *, /, %
		if len(mutations) != 4 {
			t.Fatalf("expected 4 mutations, got %d", len(mutations))
		}

		// Verify mutations
		expectedOps := map[token.Token]bool{
			token.SUB: false,
			token.MUL: false,
			token.QUO: false,
			token.REM: false,
		}

		for _, mut := range mutations {
			if mut.Type != m.MutationArithmetic {
				t.Errorf("expected arithmetic mutation, got %v", mut.Type)
			}
			if mut.OriginalOp != token.ADD {
				t.Errorf("expected original op ADD, got %v", mut.OriginalOp)
			}
			if _, ok := expectedOps[mut.MutatedOp]; ok {
				expectedOps[mut.MutatedOp] = true
			}
		}

		for op, found := range expectedOps {
			if !found {
				t.Errorf("expected mutation to %s, but not found", op)
			}
		}
	})

	t.Run("returns empty slice when no arithmetic operators present", func(t *testing.T) {
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
			mutations = append(mutations, ProcessArithmeticMutations(n, fset, source, &mutationID)...)
			return true
		})

		if len(mutations) != 0 {
			t.Errorf("expected no mutations for non-binary expressions, got %d", len(mutations))
		}
	})
}

func TestIsArithmeticOp(t *testing.T) {
	tests := []struct {
		op       token.Token
		expected bool
	}{
		{token.ADD, true},
		{token.SUB, true},
		{token.MUL, true},
		{token.QUO, true},
		{token.REM, true},
		{token.EQL, false},
		{token.LSS, false},
		{token.GTR, false},
		{token.ILLEGAL, false},
	}

	for _, tt := range tests {
		t.Run(tt.op.String(), func(t *testing.T) {
			result := isArithmeticOp(tt.op)
			if result != tt.expected {
				t.Errorf("isArithmeticOp(%s) = %v, expected %v", tt.op, result, tt.expected)
			}
		})
	}
}

func TestGetArithmeticAlternatives(t *testing.T) {
	tests := []struct {
		original token.Token
		expected []token.Token
	}{
		{
			token.ADD,
			[]token.Token{token.SUB, token.MUL, token.QUO, token.REM},
		},
		{
			token.SUB,
			[]token.Token{token.ADD, token.MUL, token.QUO, token.REM},
		},
		{
			token.MUL,
			[]token.Token{token.ADD, token.SUB, token.QUO, token.REM},
		},
	}

	for _, tt := range tests {
		t.Run(tt.original.String(), func(t *testing.T) {
			result := getArithmeticAlternatives(tt.original)
			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d alternatives, got %d", len(tt.expected), len(result))
			}

			expectedMap := make(map[token.Token]bool)
			for _, op := range tt.expected {
				expectedMap[op] = true
			}

			for _, op := range result {
				if !expectedMap[op] {
					t.Errorf("unexpected alternative: %s", op)
				}
			}
		})
	}
}
