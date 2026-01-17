package mutagens

import (
	"go/ast"
	"go/token"

	m "github.com/mouse-blink/gooze/internal/model"
)

// GenerateComparisonMutations generates comparison operator mutations for the given AST node.
func GenerateComparisonMutations(n ast.Node, fset *token.FileSet, content []byte, source m.Source, mutationID *int) []m.Mutation {
	binExpr, ok := n.(*ast.BinaryExpr)
	if !ok {
		return nil
	}

	if !isComparisonOp(binExpr.Op) {
		return nil
	}

	start, ok := offsetForPos(fset, binExpr.OpPos)
	if !ok {
		return nil
	}

	original := binExpr.Op.String()
	end := start + len(original)

	var mutations []m.Mutation

	for _, mutatedOp := range getComparisonAlternatives(binExpr.Op) {
		*mutationID++
		mutatedCode := replaceRange(content, start, end, mutatedOp.String())
		mutations = append(mutations, m.Mutation{
			ID:          *mutationID - 1,
			Source:      source,
			Type:        m.MutationComparison,
			MutatedCode: mutatedCode,
		})
	}

	return mutations
}

func isComparisonOp(op token.Token) bool {
	return op == token.LSS || op == token.GTR || op == token.LEQ ||
		op == token.GEQ || op == token.EQL || op == token.NEQ
}

func getComparisonAlternatives(original token.Token) []token.Token {
	allOps := []token.Token{token.LSS, token.GTR, token.LEQ, token.GEQ, token.EQL, token.NEQ}

	var alternatives []token.Token

	for _, op := range allOps {
		if op != original {
			alternatives = append(alternatives, op)
		}
	}

	return alternatives
}
