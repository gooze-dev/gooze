package mutagens

import (
	"fmt"
	"go/ast"
	"go/token"

	m "github.com/mouse-blink/gooze/internal/model"
)

const (
	trueStr  = "true"
	falseStr = "false"
)

// ProcessBooleanMutations generates boolean mutations for a node.
func ProcessBooleanMutations(n ast.Node, fset *token.FileSet, source m.Source, mutationID *int) []m.Mutation {
	ident, ok := n.(*ast.Ident)
	if !ok {
		return nil
	}

	if !isBooleanLiteral(ident.Name) {
		return nil
	}

	pos := fset.Position(ident.Pos())

	scopeType := FindScopeType(source.Scopes, pos.Line)
	*mutationID++
	original := ident.Name
	mutated := flipBoolean(original)

	return []m.Mutation{{
		ID:           fmt.Sprintf("BOOL_%d", *mutationID),
		Type:         m.MutationBoolean,
		SourceFile:   source.Origin,
		OriginalOp:   token.ILLEGAL, // Not used for boolean mutations
		MutatedOp:    token.ILLEGAL, // Not used for boolean mutations
		OriginalText: original,
		MutatedText:  mutated,
		Line:         pos.Line,
		Column:       pos.Column,
		ScopeType:    scopeType,
	}}
}

// isBooleanLiteral checks if a string is a boolean literal.
func isBooleanLiteral(name string) bool {
	return name == trueStr || name == falseStr
}

// flipBoolean returns the opposite boolean literal.
func flipBoolean(original string) string {
	if original == trueStr {
		return falseStr
	}

	return trueStr
}
