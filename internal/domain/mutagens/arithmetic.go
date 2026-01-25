// Package mutagens provides mutation generators for different mutation types.
package mutagens

import (
	"go/ast"
	"go/token"

	m "github.com/mouse-blink/gooze/internal/model"
)

// GenerateArithmeticMutations generates arithmetic mutations for the given AST node.
func GenerateArithmeticMutations(n ast.Node, fset *token.FileSet, content []byte, source m.Source) []m.Mutation {
	return generateBinaryExprMutations(n, fset, content, source, m.MutationArithmetic, isArithmeticOp, getArithmeticAlternatives)
}

func isArithmeticOp(op token.Token) bool {
	return op == token.ADD || op == token.SUB || op == token.MUL || op == token.QUO || op == token.REM
}

func getArithmeticAlternatives(original token.Token) []token.Token {
	allOps := []token.Token{token.ADD, token.SUB, token.MUL, token.QUO, token.REM}

	var alternatives []token.Token

	for _, op := range allOps {
		if op != original {
			alternatives = append(alternatives, op)
		}
	}

	return alternatives
}
