package mutagens

import (
	"go/ast"
	"go/token"

	m "github.com/mouse-blink/gooze/internal/model"
)

// GenerateComparisonMutations generates comparison operator mutations for the given AST node.
func GenerateComparisonMutations(n ast.Node, fset *token.FileSet, content []byte, source m.Source) []m.Mutation {
	return generateBinaryExprMutations(n, fset, content, source, m.MutationComparison, isComparisonOp, getComparisonAlternatives)
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
