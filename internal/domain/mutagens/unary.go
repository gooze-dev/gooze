package mutagens

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/token"

	m "github.com/mouse-blink/gooze/internal/model"
)

// GenerateUnaryMutations generates unary operator mutations for the given AST node.
func GenerateUnaryMutations(n ast.Node, fset *token.FileSet, content []byte, source m.Source) []m.Mutation {
	unaryExpr, ok := n.(*ast.UnaryExpr)
	if !ok {
		return nil
	}

	if !isUnaryOp(unaryExpr.Op) {
		return nil
	}

	start, ok := offsetForPos(fset, unaryExpr.OpPos)
	if !ok {
		return nil
	}

	original := unaryExpr.Op.String()
	end := start + len(original)

	var mutations []m.Mutation

	for _, mutatedOp := range getUnaryAlternatives(unaryExpr.Op) {
		mutatedCode := replaceRange(content, start, end, mutatedOp.String())
		diff := diffCode(content, mutatedCode)
		h := sha256.Sum256(mutatedCode)
		id := fmt.Sprintf("%x", h)
		mutations = append(mutations, m.Mutation{
			ID:          id,
			Source:      source,
			Type:        m.MutationUnary,
			MutatedCode: mutatedCode,
			DiffCode:    diff,
		})
	}

	// Also generate removal mutation (remove the unary operator entirely)
	mutatedCode := replaceRange(content, start, end, "")
	diff := diffCode(content, mutatedCode)
	h := sha256.Sum256(mutatedCode)
	id := fmt.Sprintf("%x", h)
	mutations = append(mutations, m.Mutation{
		ID:          id,
		Source:      source,
		Type:        m.MutationUnary,
		MutatedCode: mutatedCode,
		DiffCode:    diff,
	})

	return mutations
}

func isUnaryOp(op token.Token) bool {
	return op == token.SUB || op == token.ADD || op == token.NOT || op == token.XOR
}

func getUnaryAlternatives(original token.Token) []token.Token {
	switch original { //nolint:exhaustive
	case token.SUB: // -x
		return []token.Token{token.ADD} // +x
	case token.ADD: // +x
		return []token.Token{token.SUB} // -x
	case token.NOT: // !x
		return []token.Token{} // No direct alternative for logical NOT, only removal
	case token.XOR: // ^x (bitwise NOT)
		return []token.Token{} // No direct alternative for bitwise NOT, only removal
	default:
		return []token.Token{}
	}
}
