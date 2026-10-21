package mutagens

import (
	"go/ast"
	"go/token"

	m "gooze.dev/pkg/gooze/internal/model"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// GenerateBranchMutations generates branch mutations for the given AST node:
// an if statement's condition is inverted and forced true and false, its
// block and its else block are removed; a for loop's condition is inverted
// and forced false (forcing it true would loop until the test times out);
// each case body of a switch is removed.
func GenerateBranchMutations(n ast.Node, fset *token.FileSet, content []byte, source rpt.Source) []m.Mutation {
	switch stmt := n.(type) {
	case *ast.IfStmt:
		return mutateIfStatement(stmt, fset, content, source)
	case *ast.ForStmt:
		if stmt.Cond != nil {
			return invertCondition(stmt.Cond, fset, content, source, "!", "false")
		}
	case *ast.SwitchStmt:
		return mutateSwitchStatement(stmt, fset, content, source)
	}

	return nil
}

// mutateIfStatement creates comprehensive mutations for if statements.
func mutateIfStatement(stmt *ast.IfStmt, fset *token.FileSet, content []byte, source rpt.Source) []m.Mutation {
	var mutations []m.Mutation

	// 1. Mutate the condition (invert, force true, force false). Forcing it
	// would leave unused a variable the init statement declares for the
	// condition alone, as ok in if v, ok := m[k]; ok.
	if stmt.Cond != nil {
		replacements := []string{"!", "true", "false"}
		if declaresForCondOnly(stmt) {
			replacements = replacements[:1]
		}

		mutations = append(mutations, invertCondition(stmt.Cond, fset, content, source, replacements...)...)
	}

	// 2. Remove the if block (keep only else if it exists)
	mutations = append(mutations, removeIfBlock(stmt, fset, content, source)...)

	// 3. Remove the else block (keep only if)
	if stmt.Else != nil {
		mutations = append(mutations, removeElseBlock(stmt, fset, content, source)...)
	}

	return mutations
}

// declaresForCondOnly reports whether the init statement of an if declares a
// variable that only the condition uses.
func declaresForCondOnly(stmt *ast.IfStmt) bool {
	assign, ok := stmt.Init.(*ast.AssignStmt)
	// Equivalent: every other assignment token sorts before DEFINE.
	if !ok || assign.Tok != token.DEFINE { //gooze:ignore comparison
		return false
	}

	used := map[string]bool{}
	markUsed := func(n ast.Node) bool {
		if ident, ok := n.(*ast.Ident); ok {
			used[ident.Name] = true
		}

		return true
	}

	ast.Inspect(stmt.Body, markUsed)

	if stmt.Else != nil {
		ast.Inspect(stmt.Else, markUsed)
	}

	for _, lhs := range assign.Lhs {
		if ident, ok := lhs.(*ast.Ident); ok && ident.Name != "_" && !used[ident.Name] {
			return true
		}
	}

	return false
}

// removeIfBlock creates a mutation that removes the if block, keeping the else block if present.
func removeIfBlock(stmt *ast.IfStmt, fset *token.FileSet, content []byte, source rpt.Source) []m.Mutation {
	offset, ok := offsetForPos(fset, stmt.Pos())
	if !ok {
		return nil
	}

	endOffset, ok := offsetForPos(fset, stmt.End())
	if !ok {
		return nil
	}

	var mutated []byte
	if stmt.Else != nil {
		mutated = replaceIfWithElse(stmt, fset, content, offset, endOffset)
	} else {
		mutated = deleteRange(content, offset, endOffset)
	}

	if mutated == nil {
		return nil
	}

	return []m.Mutation{branchMutation(mutated, source)}
}

// replaceIfWithElse replaces an if statement with its else block content.
func replaceIfWithElse(stmt *ast.IfStmt, fset *token.FileSet, content []byte, offset, endOffset int) []byte {
	elseOffset, ok := offsetForPos(fset, stmt.Else.Pos())
	if !ok {
		return nil
	}

	elseContent := extractElseContent(stmt.Else, fset, content, elseOffset, endOffset)
	if elseContent == "" {
		return nil
	}

	return replaceRange(content, offset, endOffset, elseContent)
}

// extractElseContent extracts the content from an else block.
func extractElseContent(elseNode ast.Node, fset *token.FileSet, content []byte, elseOffset, endOffset int) string {
	switch elseStmt := elseNode.(type) {
	case *ast.BlockStmt:
		return extractBlockBody(elseStmt, fset, content)
	case *ast.IfStmt:
		return string(content[elseOffset:endOffset])
	default:
		return ""
	}
}

// extractBlockBody extracts the body content from a block statement.
func extractBlockBody(block *ast.BlockStmt, fset *token.FileSet, content []byte) string {
	bodyStart, ok1 := offsetForPos(fset, block.Lbrace)

	bodyEnd, ok2 := offsetForPos(fset, block.Rbrace)
	if ok1 && ok2 {
		return string(content[bodyStart+1 : bodyEnd])
	}

	return ""
}

// branchMutation returns the branch mutation that turns the file into mutated.
func branchMutation(mutated []byte, source rpt.Source) m.Mutation {
	mutated = ensureTrailingNewline(mutated)

	return m.Mutation{
		ID:          mutationID(mutated),
		Source:      source,
		Type:        mgen.Branch,
		MutatedCode: mutated,
	}
}

// removeElseBlock creates a mutation that removes the else block.
func removeElseBlock(stmt *ast.IfStmt, fset *token.FileSet, content []byte, source rpt.Source) []m.Mutation {
	if stmt.Else == nil {
		return nil
	}

	ifEndOffset, ok1 := offsetForPos(fset, stmt.Body.End())
	elseEndOffset, ok2 := offsetForPos(fset, stmt.Else.End())

	// Equivalent: both positions are in the file being mutated, so neither
	// lookup fails.
	if !ok1 || !ok2 { //gooze:ignore logical
		return nil
	}

	// Find the "else" keyword position (between if block end and else block start)
	// We need to remove from after the if block's closing } to the end of else block
	elseKeywordStart := ifEndOffset
	for elseKeywordStart < len(content) && content[elseKeywordStart] != 'e' {
		elseKeywordStart++
	}

	mutated := replaceRange(content, elseKeywordStart, elseEndOffset, "")

	return []m.Mutation{branchMutation(mutated, source)}
}

// mutateSwitchStatement creates mutations for switch statements and their cases.
func mutateSwitchStatement(stmt *ast.SwitchStmt, fset *token.FileSet, content []byte, source rpt.Source) []m.Mutation {
	var mutations []m.Mutation

	// Mutate each case clause
	if stmt.Body != nil {
		for _, caseStmt := range stmt.Body.List {
			if clause, ok := caseStmt.(*ast.CaseClause); ok {
				mutations = append(mutations, mutateCaseClause(clause, fset, content, source)...)
			}
		}
	}

	return mutations
}

// mutateCaseClause creates mutations for individual case clauses.
func mutateCaseClause(clause *ast.CaseClause, fset *token.FileSet, content []byte, source rpt.Source) []m.Mutation {
	var mutations []m.Mutation

	// Remove the entire case clause (body)
	if len(clause.Body) > 0 {
		mutations = append(mutations, removeCaseBody(clause, fset, content, source)...)
	}

	// For non-default cases with expressions, we could mutate the case values
	// but that's complex and overlaps with other mutation types

	return mutations
}

// removeCaseBody creates a mutation that removes the case body.
func removeCaseBody(clause *ast.CaseClause, fset *token.FileSet, content []byte, source rpt.Source) []m.Mutation {
	if len(clause.Body) == 0 {
		return nil
	}

	// Find the colon after case/default keyword
	colonPos := clause.Colon
	colonOffset, ok1 := offsetForPos(fset, colonPos)

	// Find the end of the case body
	lastStmt := clause.Body[len(clause.Body)-1]
	endPos := lastStmt.End()
	endOffset, ok2 := offsetForPos(fset, endPos)

	if !ok1 || !ok2 {
		return nil
	}

	// Replace case body with empty body (keep the colon, remove statements)
	// Move to position after colon
	bodyStart := colonOffset + 1

	mutated := replaceRange(content, bodyStart, endOffset, "")

	return []m.Mutation{branchMutation(mutated, source)}
}

// invertCondition returns a mutation of cond for each replacement: "!"
// inverts it, any other replacement takes its place unless cond already reads
// so (forcing "true" true changes nothing).
func invertCondition(cond ast.Expr, fset *token.FileSet, content []byte, source rpt.Source, replacements ...string) []m.Mutation {
	offset, ok1 := offsetForPos(fset, cond.Pos())

	endOffset, ok2 := offsetForPos(fset, cond.End())
	if !ok1 || !ok2 {
		return nil
	}

	original := string(content[offset:endOffset])
	mutations := make([]m.Mutation, 0, len(replacements))

	for _, replacement := range replacements {
		if replacement == original {
			continue
		}

		// Equivalent: no other replacement sorts before "!".
		if replacement == "!" { //gooze:ignore comparison
			replacement = "!(" + original + ")"
		}

		mutations = append(mutations, branchMutation(replaceRange(content, offset, endOffset, replacement), source))
	}

	return mutations
}
