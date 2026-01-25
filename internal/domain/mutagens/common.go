// Package mutagens provides functions to generate code mutations.
package mutagens

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/token"
	"strings"

	m "github.com/mouse-blink/gooze/internal/model"
	"github.com/pmezard/go-difflib/difflib"
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

func diffCode(original []byte, mutated []byte) []byte {
	if len(original) == 0 && len(mutated) == 0 {
		return nil
	}

	ud := difflib.UnifiedDiff{
		FromFile: "original",
		ToFile:   "mutated",
		Context:  3,
		A:        difflib.SplitLines(string(ensureTrailingNewline(original))),
		B:        difflib.SplitLines(string(ensureTrailingNewline(mutated))),
	}

	text, err := difflib.GetUnifiedDiffString(ud)
	if err != nil {
		return nil
	}

	return []byte(text)
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

	for _, mutatedOp := range getAlternatives(binExpr.Op) {
		mutatedCode := replaceRange(content, start, end, mutatedOp.String())
		diff := diffCode(content, mutatedCode)
		h := sha256.Sum256(mutatedCode)
		id := fmt.Sprintf("%x", h)
		mutations = append(mutations, m.Mutation{
			ID:          id,
			Source:      source,
			Type:        mutationType,
			MutatedCode: mutatedCode,
			DiffCode:    diff,
		})
	}

	return mutations
}
