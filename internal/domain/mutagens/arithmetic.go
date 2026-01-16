// Package mutagens provides mutation generators for different mutation types.
package mutagens

import (
	"go/ast"
	"go/token"

	m "github.com/mouse-blink/gooze/internal/model"
)

// GenerateArithmeticMutations generates arithmetic mutations for the given AST node.
func GenerateArithmeticMutations(n ast.Node, fset *token.FileSet, content []byte, source m.Source, mutationID *int) []m.Mutation {
	binExpr, ok := n.(*ast.BinaryExpr)
	if !ok {
		return nil
	}

	if !isArithmeticOp(binExpr.Op) {
		return nil
	}

	start, ok := offsetForPos(fset, binExpr.OpPos)
	if !ok {
		return nil
	}

	original := binExpr.Op.String()
	end := start + len(original)

	var mutations []m.Mutation

	for _, mutatedOp := range getArithmeticAlternatives(binExpr.Op) {
		*mutationID++
		mutatedCode := replaceRange(content, start, end, mutatedOp.String())
		mutations = append(mutations, m.Mutation{
			ID:          *mutationID - 1,
			Source:      source,
			Type:        m.MutationArithmetic,
			MutatedCode: mutatedCode,
		})
	}

	return mutations
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
