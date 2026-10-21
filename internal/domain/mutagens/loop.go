package mutagens

import (
	"go/ast"
	"go/token"

	m "gooze.dev/pkg/gooze/internal/model"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// GenerateLoopMutations generates loop mutations for the given AST node.
// Loop mutations test loop boundaries, loop body execution, and control flow.
// None of them makes a loop that ended run forever, since the test would only
// time out: a body is emptied only when a post statement or a range still
// ends the loop, and a break is removed only from a loop with a condition or
// a range.
func GenerateLoopMutations(n ast.Node, fset *token.FileSet, content []byte, source rpt.Source) []m.Mutation {
	switch stmt := n.(type) {
	case *ast.ForStmt:
		return mutateForLoop(stmt, fset, content, source)
	case *ast.RangeStmt:
		return mutateRangeLoop(stmt, fset, content, source)
	case *ast.FuncDecl:
		// Detect and mutate recursive calls within functions
		return mutateRecursiveCalls(stmt, fset, content, source)
	}

	return nil
}

// mutateForLoop creates mutations for loops.
func mutateForLoop(stmt *ast.ForStmt, fset *token.FileSet, content []byte, source rpt.Source) []m.Mutation {
	var mutations []m.Mutation

	// Mutate loop boundary conditions
	if stmt.Cond != nil {
		mutations = append(mutations, mutateLoopBoundary(stmt.Cond, fset, content, source)...)
	}

	// Remove loop body
	if stmt.Post != nil && len(stmt.Body.List) > 0 {
		mutations = append(mutations, removeLoopBody(stmt.Body, fset, content, source)...)
	}

	for _, branch := range loopControls(stmt.Body, stmt.Cond != nil) {
		mutations = append(mutations, removeBranchStatement(branch, fset, content, source)...)
	}

	return mutations
}

// mutateRangeLoop creates mutations for range loops.
func mutateRangeLoop(stmt *ast.RangeStmt, fset *token.FileSet, content []byte, source rpt.Source) []m.Mutation {
	var mutations []m.Mutation

	if len(stmt.Body.List) > 0 && !declaresNamed(stmt) {
		mutations = append(mutations, removeLoopBody(stmt.Body, fset, content, source)...)
	}

	for _, branch := range loopControls(stmt.Body, true) {
		mutations = append(mutations, removeBranchStatement(branch, fset, content, source)...)
	}

	return mutations
}

// declaresNamed reports whether a range loop declares a key or value other
// than _: emptying its body would leave the variable unused, which does not
// compile.
func declaresNamed(stmt *ast.RangeStmt) bool {
	// Equivalent: a range's other tokens, = and none, sort before DEFINE.
	if stmt.Tok != token.DEFINE { //gooze:ignore comparison
		return false
	}

	for _, expr := range []ast.Expr{stmt.Key, stmt.Value} {
		if ident, ok := expr.(*ast.Ident); ok && ident.Name != "_" {
			return true
		}
	}

	return false
}

// loopControls returns the unlabeled continue statements of a loop's body,
// and its unlabeled break statements when withBreaks is set. Statements of
// nested loops and function literals belong to those, and so do the breaks
// of a nested switch or select; labeled ones are left out because removing
// one can leave its label unused.
func loopControls(body *ast.BlockStmt, withBreaks bool) []*ast.BranchStmt {
	var found []*ast.BranchStmt

	var visit func(root ast.Node, inSwitch bool)

	visit = func(root ast.Node, inSwitch bool) {
		ast.Inspect(root, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.FuncLit, *ast.ForStmt, *ast.RangeStmt:
				return false
			case *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
				if node != root {
					visit(node, true)
					return false
				}
			case *ast.BranchStmt:
				// Equivalent: no branch token sorts before BREAK, and the ones
				// after it are continue (matched already), goto (always
				// labeled) and fallthrough (always in a switch).
				isBreak := node.Tok == token.BREAK //gooze:ignore comparison
				if node.Label == nil && (node.Tok == token.CONTINUE || isBreak && withBreaks && !inSwitch) {
					found = append(found, node)
				}
			}

			return true
		})
	}

	visit(body, false)

	return found
}

// loopMutation returns the loop mutation that turns the file into mutated.
func loopMutation(mutated []byte, source rpt.Source) m.Mutation {
	mutated = ensureTrailingNewline(mutated)

	return m.Mutation{
		ID:          mutationID(mutated),
		Source:      source,
		Type:        mgen.Loop,
		MutatedCode: mutated,
	}
}

// mutateLoopBoundary mutates loop boundary conditions to test off-by-one errors.
func mutateLoopBoundary(cond ast.Expr, fset *token.FileSet, content []byte, source rpt.Source) []m.Mutation {
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
		mutations = append(mutations, loopMutation(replaceRange(content, opStart, opEnd, mutatedOp.String()), source))
	}

	return mutations
}

// removeLoopBody creates a mutation that empties a loop body.
func removeLoopBody(body *ast.BlockStmt, fset *token.FileSet, content []byte, source rpt.Source) []m.Mutation {
	bodyStart, ok1 := offsetForPos(fset, body.Lbrace)
	bodyEnd, ok2 := offsetForPos(fset, body.Rbrace)

	if !ok1 || !ok2 {
		return nil
	}

	return []m.Mutation{loopMutation(replaceRange(content, bodyStart+1, bodyEnd, ""), source)}
}

// removeBranchStatement creates a mutation that removes break or continue statements.
func removeBranchStatement(stmt *ast.BranchStmt, fset *token.FileSet, content []byte, source rpt.Source) []m.Mutation {
	offset, ok1 := offsetForPos(fset, stmt.Pos())
	endOffset, ok2 := offsetForPos(fset, stmt.End())

	if !ok1 || !ok2 {
		return nil
	}

	return []m.Mutation{loopMutation(deleteRange(content, offset, endOffset), source)}
}

// isLoopBoundaryOp checks if the token is a loop boundary operator.
func isLoopBoundaryOp(op token.Token) bool {
	return op == token.LSS || op == token.LEQ || op == token.GTR || op == token.GEQ
}

// getLoopBoundaryAlternatives returns alternative operators for boundary testing.
func getLoopBoundaryAlternatives(original token.Token) []token.Token {
	switch original {
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
// Only functions that return one number are mutated, since 0 must take the
// call's place; in a method, Name() calls a function, not the method.
func mutateRecursiveCalls(funcDecl *ast.FuncDecl, fset *token.FileSet, content []byte, source rpt.Source) []m.Mutation {
	if funcDecl.Body == nil || funcDecl.Recv != nil || !returnsOneNumber(funcDecl.Type) {
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

// numberTypes are the predeclared types 0 converts to.
var numberTypes = map[string]bool{
	"int": true, "int8": true, "int16": true, "int32": true, "int64": true,
	"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true, "uintptr": true,
	"float32": true, "float64": true, "complex64": true, "complex128": true, "byte": true, "rune": true,
}

// returnsOneNumber reports whether a function returns a single value of a
// predeclared number type.
func returnsOneNumber(fn *ast.FuncType) bool {
	if fn.Results == nil || len(fn.Results.List) != 1 || len(fn.Results.List[0].Names) > 1 {
		return false
	}

	ident, ok := fn.Results.List[0].Type.(*ast.Ident)

	return ok && numberTypes[ident.Name]
}

// findRecursiveInExpr finds recursive calls in an expression.
func findRecursiveInExpr(expr ast.Expr, funcName string, fset *token.FileSet, content []byte, source rpt.Source) []m.Mutation {
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
func removeRecursiveCallExpr(call *ast.CallExpr, fset *token.FileSet, content []byte, source rpt.Source) *m.Mutation {
	offset, ok1 := offsetForPos(fset, call.Pos())
	endOffset, ok2 := offsetForPos(fset, call.End())

	if !ok1 || !ok2 {
		return nil
	}

	// Replace the call with a default value (0 for simplicity)
	mutation := loopMutation(replaceRange(content, offset, endOffset, "0"), source)

	return &mutation
}
