// Package mutagens provides functions to generate code mutations.
package mutagens

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/token"
	"strings"

	m "gooze.dev/pkg/gooze/internal/model"
)

func offsetForPos(fset *token.FileSet, pos token.Pos) (int, bool) {
	file := fset.File(pos)
	if file == nil {
		return 0, false
	}

	return file.Offset(pos), true
}

func replaceRange(content []byte, start, end int, replacement string) []byte {
	if start < 0 || end < start || end > len(content) {
		return content
	}

	mutated := make([]byte, 0, len(content)-(end-start)+len(replacement))
	mutated = append(mutated, content[:start]...)
	mutated = append(mutated, []byte(replacement)...)
	mutated = append(mutated, content[end:]...)

	return mutated
}

func ensureTrailingNewline(content []byte) []byte {
	if len(content) == 0 || strings.HasSuffix(string(content), "\n") {
		return content
	}

	return append(content, '\n')
}

// generateBinaryExprMutations is a common function to generate mutations for binary expressions.
func generateBinaryExprMutations(
	n ast.Node,
	fset *token.FileSet,
	content []byte,
	source m.Source,
	mutationType m.MutationType,
	isValidOp func(token.Token) bool,
	getAlternatives func(token.Token) []token.Token,
) []m.Mutation {
	binExpr, ok := n.(*ast.BinaryExpr)
	if !ok {
		return nil
	}

	if !isValidOp(binExpr.Op) {
		return nil
	}

	start, ok := offsetForPos(fset, binExpr.OpPos)
	if !ok {
		return nil
	}

	original := binExpr.Op.String()
	end := start + len(original)

	var mutations []m.Mutation

	for _, mutatedOp := range viableOps(binExpr, getAlternatives(binExpr.Op)) {
		mutatedCode := replaceRange(content, start, end, mutatedOp.String())
		h := sha256.Sum256(mutatedCode)
		id := fmt.Sprintf("%x", h)
		mutations = append(mutations, m.Mutation{
			ID:          id,
			Source:      source,
			Type:        mutationType,
			MutatedCode: mutatedCode,
		})
	}

	return mutations
}

// viableOps drops operator alternatives that cannot compile for the given
// operands: nil may only be compared with == and !=, and string literals only
// support +.
func viableOps(expr *ast.BinaryExpr, ops []token.Token) []token.Token {
	hasNil := isNilIdent(expr.X) || isNilIdent(expr.Y)
	hasString := isStringLit(expr.X) || isStringLit(expr.Y)

	if !hasNil && !hasString {
		return ops
	}

	viable := make([]token.Token, 0, len(ops))

	for _, op := range ops {
		if opCompiles(op, hasNil, hasString) {
			viable = append(viable, op)
		}
	}

	return viable
}

func opCompiles(op token.Token, hasNil, hasString bool) bool {
	if hasNil && op != token.EQL && op != token.NEQ {
		return false
	}

	return !hasString || !isArithmeticOp(op) || op == token.ADD
}

func isNilIdent(e ast.Expr) bool {
	ident, ok := e.(*ast.Ident)

	return ok && ident.Name == "nil"
}

func isStringLit(e ast.Expr) bool {
	lit, ok := e.(*ast.BasicLit)

	return ok && lit.Kind == token.STRING
}
