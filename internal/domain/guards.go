package domain

import (
	"go/ast"
	"go/token"

	m "gooze.dev/pkg/gooze/internal/model"
)

// guard is a condition, from the offset start up to end, and where the code
// starts that only runs through it.
type guard struct {
	start, end int
	guards     m.Position
}

// guardIndex is the conditions of a file, an outer one before those inside it.
type guardIndex []guard

// findGuards lists the conditions of file that guard code: that of an if or
// a for statement with its body, and the expressions of a case clause with
// its statements. A condition that guards no statement is left out, as a
// coverage profile knows nothing of an empty block.
func findGuards(file *ast.File, tokenFile *token.File) guardIndex {
	if tokenFile == nil {
		return nil
	}

	var index guardIndex

	add := func(first, last ast.Expr, guarded []ast.Stmt) {
		if first == nil || len(guarded) == 0 {
			return
		}

		// Unadjusted, as a profile's positions are: the tests of a file with
		// //line comments are still compiled from the file itself.
		position := tokenFile.PositionFor(guarded[0].Pos(), false)

		index = append(index, guard{
			start:  tokenFile.Offset(first.Pos()),
			end:    tokenFile.Offset(last.End()),
			guards: m.Position{Line: position.Line, Column: position.Column},
		})
	}

	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.IfStmt:
			add(node.Cond, node.Cond, node.Body.List)
		case *ast.ForStmt:
			add(node.Cond, node.Cond, node.Body.List)
		case *ast.CaseClause:
			if len(node.List) > 0 {
				add(node.List[0], node.List[len(node.List)-1], node.Body)
			}
		}

		return true
	})

	return index
}

// at returns what the condition at offset guards: the innermost one, for a
// condition inside a function literal of another. Nil outside any.
func (ix guardIndex) at(offset int) *m.Position {
	var found *m.Position

	for _, g := range ix {
		if g.start <= offset && offset < g.end {
			found = &m.Position{Line: g.guards.Line, Column: g.guards.Column}
		}
	}

	return found
}

// mark returns fn with each mutation told what it guards, by where it
// changes content.
func (ix guardIndex) mark(content []byte, fn func(m.Mutation) error) func(m.Mutation) error {
	return func(mutation m.Mutation) error {
		mutation.Guards = ix.at(firstDifference(content, mutation.MutatedCode))

		return fn(mutation)
	}
}
