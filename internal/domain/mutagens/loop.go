package mutagens

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/token"

	m "github.com/mouse-blink/gooze/internal/model"
)

// GenerateLoopMutations generates loop mutations for the given AST node.
// Loop mutations test loop boundaries, loop body execution, and control flow.
func GenerateLoopMutations(n ast.Node, fset *token.FileSet, content []byte, source m.Source) []m.Mutation {
	var mutations []m.Mutation

	switch stmt := n.(type) {
	case *ast.ForStmt:
		mutations = append(mutations, mutateForLoop(stmt, fset, content, source)...)
	case *ast.RangeStmt:
		mutations = append(mutations, mutateRangeLoop(stmt, fset, content, source)...)
	case *ast.BranchStmt:
		if stmt.Tok == token.BREAK || stmt.Tok == token.CONTINUE {
			mutations = append(mutations, removeBranchStatement(stmt, fset, content, source)...)
		}
	case *ast.FuncDecl:
		// Detect and mutate recursive calls within functions
		mutations = append(mutations, mutateRecursiveCalls(stmt, fset, content, source)...)
	}

	return mutations
}

// mutateForLoop creates mutations for loops.
func mutateForLoop(stmt *ast.ForStmt, fset *token.FileSet, content []byte, source m.Source) []m.Mutation {
	var mutations []m.Mutation

	// Mutate loop boundary conditions
	if stmt.Cond != nil {
		mutations = append(mutations, mutateLoopBoundary(stmt.Cond, fset, content, source)...)
	}

	// Remove loop body
	if stmt.Body != nil && len(stmt.Body.List) > 0 {
		mutations = append(mutations, removeForLoopBody(stmt, fset, content, source)...)
	}

	return mutations
}

// mutateRangeLoop creates mutations for range loops.
func mutateRangeLoop(stmt *ast.RangeStmt, fset *token.FileSet, content []byte, source m.Source) []m.Mutation {
	if stmt.Body == nil || len(stmt.Body.List) == 0 {
		return nil
	}

	return removeRangeLoopBody(stmt, fset, content, source)
}

// mutateLoopBoundary mutates loop boundary conditions to test off-by-one errors.
func mutateLoopBoundary(cond ast.Expr, fset *token.FileSet, content []byte, source m.Source) []m.Mutation {
	binExpr, ok := cond.(*ast.BinaryExpr)
	if !ok {
		return nil
	}

	if !isLoopBoundaryOp(binExpr.Op) {
		return nil
	}

	opStart, ok := offsetForPos(fset, binExpr.OpPos)
	if !ok {
		return nil
	}

	original := binExpr.Op.String()
	opEnd := opStart + len(original)

	alternatives := getLoopBoundaryAlternatives(binExpr.Op)
	if len(alternatives) == 0 {
		return nil
	}

	mutations := make([]m.Mutation, 0, len(alternatives))
	for _, mutatedOp := range alternatives {
		mutated := replaceRange(content, opStart, opEnd, mutatedOp.String())
		diff := diffCode(content, mutated)

		h := sha256.Sum256([]byte(fmt.Sprintf("%s-%s-boundary-%d", source.Origin.FullPath, m.MutationLoop.Name, opStart)))
		id := fmt.Sprintf("%x", h)[:16]

		mutations = append(mutations, m.Mutation{
			ID:          id,
			Source:      source,
			Type:        m.MutationLoop,
			MutatedCode: ensureTrailingNewline(mutated),
			DiffCode:    diff,
		})
	}

	return mutations
}

// removeForLoopBody creates a mutation that removes the for loop body.
func removeForLoopBody(stmt *ast.ForStmt, fset *token.FileSet, content []byte, source m.Source) []m.Mutation {
	bodyStart, ok1 := offsetForPos(fset, stmt.Body.Lbrace)
	bodyEnd, ok2 := offsetForPos(fset, stmt.Body.Rbrace)

	if !ok1 || !ok2 {
		return nil
	}

	mutated := replaceRange(content, bodyStart+1, bodyEnd, "")
	diff := diffCode(content, mutated)

	h := sha256.Sum256([]byte(fmt.Sprintf("%s-%s-body-%d", source.Origin.FullPath, m.MutationLoop.Name, bodyStart)))
	id := fmt.Sprintf("%x", h)[:16]

	return []m.Mutation{{
		ID:          id,
		Source:      source,
		Type:        m.MutationLoop,
		MutatedCode: ensureTrailingNewline(mutated),
		DiffCode:    diff,
	}}
}

// removeRangeLoopBody creates a mutation that removes the range loop body.
func removeRangeLoopBody(stmt *ast.RangeStmt, fset *token.FileSet, content []byte, source m.Source) []m.Mutation {
	bodyStart, ok1 := offsetForPos(fset, stmt.Body.Lbrace)
	bodyEnd, ok2 := offsetForPos(fset, stmt.Body.Rbrace)

	if !ok1 || !ok2 {
		return nil
	}

	mutated := replaceRange(content, bodyStart+1, bodyEnd, "")
	diff := diffCode(content, mutated)

	h := sha256.Sum256([]byte(fmt.Sprintf("%s-%s-range-%d", source.Origin.FullPath, m.MutationLoop.Name, bodyStart)))
	id := fmt.Sprintf("%x", h)[:16]

	return []m.Mutation{{
		ID:          id,
		Source:      source,
		Type:        m.MutationLoop,
		MutatedCode: ensureTrailingNewline(mutated),
		DiffCode:    diff,
	}}
}

// removeBranchStatement creates a mutation that removes break or continue statements.
func removeBranchStatement(stmt *ast.BranchStmt, fset *token.FileSet, content []byte, source m.Source) []m.Mutation {
	offset, ok1 := offsetForPos(fset, stmt.Pos())
	endOffset, ok2 := offsetForPos(fset, stmt.End())

	if !ok1 || !ok2 {
		return nil
	}

	// Find the end of the line
	lineEnd := endOffset
	for lineEnd < len(content) && content[lineEnd] != '\n' {
		lineEnd++
	}

	if lineEnd < len(content) {
		lineEnd++
	}

	mutated := replaceRange(content, offset, lineEnd, "")
	diff := diffCode(content, mutated)

	branchType := "break"
	if stmt.Tok == token.CONTINUE {
		branchType = "continue"
	}

	h := sha256.Sum256([]byte(fmt.Sprintf("%s-%s-%s-%d", source.Origin.FullPath, m.MutationLoop.Name, branchType, offset)))
	id := fmt.Sprintf("%x", h)[:16]

	return []m.Mutation{{
		ID:          id,
		Source:      source,
		Type:        m.MutationLoop,
		MutatedCode: ensureTrailingNewline(mutated),
		DiffCode:    diff,
	}}
}

// isLoopBoundaryOp checks if the token is a loop boundary operator.
func isLoopBoundaryOp(op token.Token) bool {
	return op == token.LSS || op == token.LEQ || op == token.GTR || op == token.GEQ
}

// getLoopBoundaryAlternatives returns alternative operators for boundary testing.
func getLoopBoundaryAlternatives(original token.Token) []token.Token {
	switch original { //nolint:exhaustive
	case token.LSS: // <
		return []token.Token{token.LEQ} // <=
	case token.LEQ: // <=
		return []token.Token{token.LSS} // <
	case token.GTR: // >
		return []token.Token{token.GEQ} // >=
	case token.GEQ: // >=
		return []token.Token{token.GTR} // >
	default:
		return nil
	}
}

// mutateRecursiveCalls finds and removes recursive calls in a function.
func mutateRecursiveCalls(funcDecl *ast.FuncDecl, fset *token.FileSet, content []byte, source m.Source) []m.Mutation {
	if funcDecl.Body == nil || funcDecl.Name == nil {
		return nil
	}

	funcName := funcDecl.Name.Name

	var mutations []m.Mutation

	// Find all recursive calls in the function body
	ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
		// Look for return statements with recursive calls
		if retStmt, ok := n.(*ast.ReturnStmt); ok {
			for _, result := range retStmt.Results {
				mutations = append(mutations, findRecursiveInExpr(result, funcName, fset, content, source)...)
			}
		}

		return true
	})

	return mutations
}

// findRecursiveInExpr finds recursive calls in an expression.
func findRecursiveInExpr(expr ast.Expr, funcName string, fset *token.FileSet, content []byte, source m.Source) []m.Mutation {
	var mutations []m.Mutation

	ast.Inspect(expr, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == funcName {
				// Found a recursive call - remove it
				mutation := removeRecursiveCallExpr(call, fset, content, source)
				if mutation != nil {
					mutations = append(mutations, *mutation)
				}
			}
		}

		return true
	})

	return mutations
}

// removeRecursiveCallExpr creates a mutation that removes a recursive call expression.
func removeRecursiveCallExpr(call *ast.CallExpr, fset *token.FileSet, content []byte, source m.Source) *m.Mutation {
	offset, ok1 := offsetForPos(fset, call.Pos())
	endOffset, ok2 := offsetForPos(fset, call.End())

	if !ok1 || !ok2 {
		return nil
	}

	// Replace the call with a default value (0 for simplicity)
	mutated := replaceRange(content, offset, endOffset, "0")
	diff := diffCode(content, mutated)

	h := sha256.Sum256([]byte(fmt.Sprintf("%s-%s-recursion-%d", source.Origin.FullPath, m.MutationLoop.Name, offset)))
	id := fmt.Sprintf("%x", h)[:16]

	return &m.Mutation{
		ID:          id,
		Source:      source,
		Type:        m.MutationLoop,
		MutatedCode: ensureTrailingNewline(mutated),
		DiffCode:    diff,
	}
}
