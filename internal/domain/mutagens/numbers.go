package mutagens

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"

	m "github.com/mouse-blink/gooze/internal/model"
)

// GenerateNumberMutations generates numeric literal mutations for the given AST node.
//
// Currently supported:
//   - token.INT:  mutate to 0 and/or 1
//   - token.FLOAT: mutate to 0.0 and/or 1.0
func GenerateNumberMutations(n ast.Node, fset *token.FileSet, content []byte, source m.Source) []m.Mutation {
	lit, ok := n.(*ast.BasicLit)
	if !ok {
		return nil
	}

	if lit.Kind != token.INT && lit.Kind != token.FLOAT {
		return nil
	}

	start, ok := offsetForPos(fset, lit.Pos())
	if !ok {
		return nil
	}

	end := start + len(lit.Value)

	alternatives := numberAlternatives(lit.Kind, lit.Value)
	if len(alternatives) == 0 {
		return nil
	}

	mutations := make([]m.Mutation, 0, len(alternatives))
	for _, alt := range alternatives {
		mutatedCode := replaceRange(content, start, end, alt)
		diff := diffCode(content, mutatedCode)
		h := sha256.Sum256(mutatedCode)
		id := fmt.Sprintf("%x", h)
		mutations = append(mutations, m.Mutation{
			ID:          id,
			Source:      source,
			Type:        m.MutationNumbers,
			MutatedCode: mutatedCode,
			DiffCode:    diff,
		})
	}

	return mutations
}

func numberAlternatives(kind token.Token, literal string) []string {
	original := constant.MakeFromLiteral(literal, kind, 0)
	if original.Kind() == constant.Unknown {
		return nil
	}

	switch kind { //nolint:exhaustive
	case token.INT:
		return numericAlternatives(original, constant.MakeInt64(0), constant.MakeInt64(1), "0", "1")
	case token.FLOAT:
		return numericAlternatives(original, constant.MakeFloat64(0), constant.MakeFloat64(1), "0.0", "1.0")
	default:
		return nil
	}
}

func numericAlternatives(original, zero, one constant.Value, zeroLit, oneLit string) []string {
	variants := make([]string, 0, 2)
	if !constant.Compare(original, token.EQL, zero) {
		variants = append(variants, zeroLit)
	}

	if !constant.Compare(original, token.EQL, one) {
		variants = append(variants, oneLit)
	}

	return variants
}
