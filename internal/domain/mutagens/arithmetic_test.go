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

func TestGenerateArithmeticMutations(t *testing.T) {
	basicPath := filepath.Join("..", "..", "..", "examples", "basic", "main.go")
	content, err := os.ReadFile(basicPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", basicPath, err)
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, basicPath, content, parser.ParseComments)
	if err != nil {
		t.Fatalf("failed to parse source %s: %v", basicPath, err)
	}

	source := m.Source{Origin: &m.File{Path: m.Path(basicPath)}}
	mutationID := 0
	var mutations []m.Mutation

	ast.Inspect(file, func(n ast.Node) bool {
		mutations = append(mutations, GenerateArithmeticMutations(n, fset, content, source, &mutationID)...)
		return true
	})

	if len(mutations) != 4 {
		t.Fatalf("expected 4 mutations, got %d", len(mutations))
	}

	expectedOps := map[string]bool{"-": false, "*": false, "/": false, "%": false}
	for i, mutation := range mutations {
		if mutation.ID != i {
			t.Fatalf("expected mutation ID %d, got %d", i, mutation.ID)
		}
		if mutation.Type != m.MutationArithmetic {
			t.Fatalf("expected arithmetic mutation, got %v", mutation.Type)
		}
		if mutation.Source.Origin == nil || mutation.Source.Origin.Path != m.Path(basicPath) {
			t.Fatalf("unexpected source origin: %+v", mutation.Source.Origin)
		}
		if bytes.Equal(mutation.MutatedCode, content) {
			t.Fatalf("expected mutated code to differ from original")
		}

		mutated := string(mutation.MutatedCode)
		for op := range expectedOps {
			if strings.Contains(mutated, "3"+op+"5") {
				expectedOps[op] = true
			}
		}
	}

	for op, found := range expectedOps {
		if !found {
			t.Errorf("expected mutation to %s, but not found", op)
		}
	}
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
			if result := isArithmeticOp(tt.op); result != tt.expected {
				t.Fatalf("isArithmeticOp(%v) = %v, expected %v", tt.op, result, tt.expected)
			}
		})
	}
}
