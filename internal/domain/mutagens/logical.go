package mutagens

import (
	"go/ast"
	"go/token"

	m "github.com/mouse-blink/gooze/internal/model"
)

// GenerateLogicalMutations generates logical operator mutations for the given AST node.
func GenerateLogicalMutations(n ast.Node, fset *token.FileSet, content []byte, source m.Source) []m.Mutation {
	return generateBinaryExprMutations(n, fset, content, source, m.MutationLogical, isLogicalOp, getLogicalAlternatives)
}

func isLogicalOp(op token.Token) bool {
	return op == token.LAND || op == token.LOR
}

func getLogicalAlternatives(original token.Token) []token.Token {
	allOps := []token.Token{token.LAND, token.LOR}

	var alternatives []token.Token

	for _, op := range allOps {
		if op != original {
			alternatives = append(alternatives, op)
		}
	}

	return alternatives
}
