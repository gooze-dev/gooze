package mutagens

import (
	"go/ast"
	"go/token"

	m "github.com/mouse-blink/gooze/internal/model"
)

func GenerateBooleanMutations(n ast.Node, fset *token.FileSet, content []byte, source m.Source, mutationID *int) []m.Mutation {
	ident, ok := n.(*ast.Ident)
	if !ok {
		return nil
	}

	if !isBooleanLiteralV2(ident.Name) {
		return nil
	}

	start, ok := offsetForPos(fset, ident.Pos())
	if !ok {
		return nil
	}

	end := start + len(ident.Name)
	mutated := flipBooleanV2(ident.Name)

	*mutationID++
	mutatedCode := replaceRange(content, start, end, mutated)
	return []m.Mutation{{
		ID:          uint(*mutationID - 1),
		Source:      source,
		Type:        m.MutationBoolean,
		MutatedCode: mutatedCode,
	}}
}

func isBooleanLiteralV2(name string) bool {
	return name == "true" || name == "false"
}

func flipBooleanV2(original string) string {
	if original == "true" {
		return "false"
	}

	return "true"
}
