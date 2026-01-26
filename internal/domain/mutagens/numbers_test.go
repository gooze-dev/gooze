package mutagens

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	m "github.com/mouse-blink/gooze/internal/model"
)

func TestGenerateNumberMutations(t *testing.T) {
	tests := []struct {
		name          string
		code          string
		expectedCount int
	}{
		{
			name:          "int literal",
			code:          "package main\nfunc f() int { return 5 }",
			expectedCount: 2, // 5 -> 0, 5 -> 1
		},
		{
			name:          "zero int literal",
			code:          "package main\nfunc f() int { return 0 }",
			expectedCount: 1, // 0 -> 1
		},
		{
			name:          "float literal",
			code:          "package main\nfunc f() float64 { return 3.14 }",
			expectedCount: 2, // 3.14 -> 0.0, 3.14 -> 1.0
		},
		{
			name:          "hex int literal",
			code:          "package main\nfunc f() int { return 0x10 }",
			expectedCount: 2, // 0x10 -> 0, 0x10 -> 1
		},
		{
			name:          "char literal is ignored",
			code:          "package main\nfunc f() rune { return 'a' }",
			expectedCount: 0,
		},
		{
			name:          "imag literal is ignored",
			code:          "package main\nvar x = 1i",
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

			source := m.Source{Origin: &m.File{FullPath: "test.go"}}

			var mutations []m.Mutation
			ast.Inspect(file, func(n ast.Node) bool {
				mutations = append(mutations, GenerateNumberMutations(n, fset, []byte(tt.code), source)...)
				return true
			})

			if len(mutations) != tt.expectedCount {
				t.Fatalf("expected %d mutations, got %d", tt.expectedCount, len(mutations))
			}

			for _, mut := range mutations {
				if mut.Type != m.MutationNumbers {
					t.Fatalf("expected mutation type %v, got %v", m.MutationNumbers, mut.Type)
				}
				if len(mut.ID) == 0 {
					t.Fatalf("expected non-empty mutation ID")
				}
			}
		})
	}
}
