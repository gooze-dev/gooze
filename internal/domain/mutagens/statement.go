package mutagens

import (
	"go/ast"
	"go/token"

	m "gooze.dev/pkg/gooze/internal/model"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// GenerateStatementMutations generates statement deletion mutations for the given AST node.
// This focuses on statement removal to test if statements are necessary.
// Deletes:
//   - Assignment statements (x = 20, x += 1); declarations (x := 10) are kept,
//     since the uses of x would not compile without them
//   - Expression statements (function calls, method calls)
//   - Defer statements
//   - Go statements (goroutine launches)
//   - Send statements (channel sends).
func GenerateStatementMutations(n ast.Node, fset *token.FileSet, content []byte, source rpt.Source) []m.Mutation {
	switch stmt := n.(type) {
	case *ast.AssignStmt:
		// Equivalent: every other assignment token sorts before DEFINE.
		if stmt.Tok != token.DEFINE { //gooze:ignore comparison
			return deleteStatement(stmt, fset, content, source)
		}

	case *ast.ExprStmt:
		return deleteStatement(stmt, fset, content, source)

	case *ast.DeferStmt:
		return deleteStatement(stmt, fset, content, source)

	case *ast.GoStmt:
		return deleteStatement(stmt, fset, content, source)

	case *ast.SendStmt:
		return deleteStatement(stmt, fset, content, source)
	}

	return nil
}

// deleteStatement creates a mutation that removes the entire statement.
func deleteStatement(stmt ast.Stmt, fset *token.FileSet, content []byte, source rpt.Source) []m.Mutation {
	offset, ok := offsetForPos(fset, stmt.Pos())
	if !ok {
		return nil
	}

	endOffset, ok := offsetForPos(fset, stmt.End())
	if !ok {
		return nil
	}

	mutated := ensureTrailingNewline(deleteRange(content, offset, endOffset))

	return []m.Mutation{{
		ID:          mutationID(mutated),
		Source:      source,
		Type:        mgen.Statement,
		MutatedCode: mutated,
	}}
}
