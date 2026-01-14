// Package mutagens provides helpers for generating code mutations.
package mutagens

import (
	"fmt"
	"go/ast"
	"go/token"

	m "github.com/mouse-blink/gooze/internal/model"
)

// ProcessArithmeticMutations generates arithmetic mutations for a node.
func ProcessArithmeticMutations(n ast.Node, fset *token.FileSet, source m.Source, mutationID *int) []m.Mutation {
	binExpr, ok := n.(*ast.BinaryExpr)
	if !ok {
		return nil
	}

	if !isArithmeticOp(binExpr.Op) {
		return nil
	}

	pos := fset.Position(binExpr.OpPos)
	scopeType := FindScopeType(source.Scopes, pos.Line)

	var mutations []m.Mutation
	// Generate mutations for all alternative operators
	for _, mutatedOp := range getArithmeticAlternatives(binExpr.Op) {
		*mutationID++
		mutations = append(mutations, m.Mutation{
			ID:         fmt.Sprintf("ARITH_%d", *mutationID),
			Type:       m.MutationArithmetic,
			SourceFile: source.Origin,
			OriginalOp: binExpr.Op,
			MutatedOp:  mutatedOp,
			Line:       pos.Line,
			Column:     pos.Column,
			ScopeType:  scopeType,
		})
	}

	return mutations
}

// isArithmeticOp checks if a token is an arithmetic operator.
func isArithmeticOp(op token.Token) bool {
	return op == token.ADD || op == token.SUB || op == token.MUL || op == token.QUO || op == token.REM
}

// getArithmeticAlternatives returns all alternative operators for mutation.
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
