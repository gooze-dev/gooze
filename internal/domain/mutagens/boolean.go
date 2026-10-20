package mutagens

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/token"

	m "gooze.dev/pkg/gooze/internal/model"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

const (
	booleanTrue  = "true"
	booleanFalse = "false"
)

// GenerateBooleanMutations generates boolean literal mutations for the given AST node.
func GenerateBooleanMutations(n ast.Node, fset *token.FileSet, content []byte, source rpt.Source) []m.Mutation {
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

	mutatedCode := replaceRange(content, start, end, mutated)
	h := sha256.Sum256(mutatedCode)
	id := fmt.Sprintf("%x", h)

	return []m.Mutation{{
		ID:          id,
		Source:      source,
		Type:        mgen.Boolean,
		MutatedCode: mutatedCode,
	}}
}

func isBooleanLiteralV2(name string) bool {
	return name == booleanTrue || name == booleanFalse
}

func flipBooleanV2(original string) string {
	if original == booleanTrue {
		return booleanFalse
	}

	return booleanTrue
}
