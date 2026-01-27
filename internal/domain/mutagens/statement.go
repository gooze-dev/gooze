package mutagens

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/token"

	m "github.com/mouse-blink/gooze/internal/model"
)

// GenerateStatementMutations generates statement deletion mutations for the given AST node.
// This focuses on statement removal to test if statements are necessary.
// Deletes:
// - Assignment statements (x := 10, x = 20)
// - Expression statements (function calls, method calls)
// - Defer statements
// - Go statements (goroutine launches)
// - Send statements (channel sends).
func GenerateStatementMutations(n ast.Node, fset *token.FileSet, content []byte, source m.Source) []m.Mutation {
	switch stmt := n.(type) {
	case *ast.AssignStmt:
		return deleteStatement(stmt, fset, content, source)

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
func deleteStatement(stmt ast.Stmt, fset *token.FileSet, content []byte, source m.Source) []m.Mutation {
	offset, ok := offsetForPos(fset, stmt.Pos())
	if !ok {
		return nil
	}

	endOffset, ok := offsetForPos(fset, stmt.End())
	if !ok {
		return nil
	}

	// Find the end of the line (including newline)
	lineEnd := endOffset
	for lineEnd < len(content) && content[lineEnd] != '\n' {
		lineEnd++
	}

	if lineEnd < len(content) {
		lineEnd++ // Include the newline
	}

	mutated := replaceRange(content, offset, lineEnd, "")
	diff := diffCode(content, mutated)

	h := sha256.Sum256([]byte(fmt.Sprintf("%s-%s-delete-%d", source.Origin.FullPath, m.MutationStatement.Name, offset)))
	id := fmt.Sprintf("%x", h)[:16]

	return []m.Mutation{{
		ID:          id,
		Source:      source,
		Type:        m.MutationStatement,
		MutatedCode: ensureTrailingNewline(mutated),
		DiffCode:    diff,
	}}
}
