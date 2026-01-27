package mutagens

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/token"

	m "github.com/mouse-blink/gooze/internal/model"
)

// GenerateBranchMutations generates branch mutations for the given AST node.
// Branch mutations modify conditional statements to test boundary behavior.
func GenerateBranchMutations(n ast.Node, fset *token.FileSet, content []byte, source m.Source) []m.Mutation {
	if n == nil {
		return nil
	}

	var mutations []m.Mutation

	ast.Inspect(n, func(node ast.Node) bool {
		if node == nil {
			return false
		}

		switch stmt := node.(type) {
		case *ast.IfStmt:
			// Mutate if statement: condition, remove if block, remove else block
			mutations = append(mutations, mutateIfStatement(stmt, fset, content, source)...)
		case *ast.ForStmt:
			// Mutate for loop condition
			if stmt.Cond != nil {
				mutations = append(mutations, invertCondition(stmt.Cond, fset, content, source)...)
			}
		case *ast.SwitchStmt:
			// Mutate switch statement and its cases
			mutations = append(mutations, mutateSwitchStatement(stmt, fset, content, source)...)
		}

		return true
	})

	return mutations
}

// mutateIfStatement creates comprehensive mutations for if statements.
func mutateIfStatement(stmt *ast.IfStmt, fset *token.FileSet, content []byte, source m.Source) []m.Mutation {
	var mutations []m.Mutation

	// 1. Mutate the condition (invert, force true, force false)
	if stmt.Cond != nil {
		mutations = append(mutations, invertCondition(stmt.Cond, fset, content, source)...)
	}

	// 2. Remove the if block (keep only else if it exists)
	mutations = append(mutations, removeIfBlock(stmt, fset, content, source)...)

	// 3. Remove the else block (keep only if)
	if stmt.Else != nil {
		mutations = append(mutations, removeElseBlock(stmt, fset, content, source)...)
	}

	return mutations
}

// removeIfBlock creates a mutation that removes the if block, keeping the else block if present.
func removeIfBlock(stmt *ast.IfStmt, fset *token.FileSet, content []byte, source m.Source) []m.Mutation {
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
		mutated = replaceRange(content, offset, endOffset, "")
	}

	if mutated == nil {
		return nil
	}

	return []m.Mutation{createIfRemovalMutation(content, mutated, source, offset)}
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

// createIfRemovalMutation creates a mutation for removing an if block.
func createIfRemovalMutation(content, mutated []byte, source m.Source, offset int) m.Mutation {
	diff := diffCode(content, mutated)
	h := sha256.Sum256([]byte(fmt.Sprintf("%s-%s-%d", source.Origin.FullPath, m.MutationBranch.Name, offset+1000)))
	id := fmt.Sprintf("%x", h)[:16]

	return m.Mutation{
		ID:          id,
		Source:      source,
		Type:        m.MutationBranch,
		MutatedCode: ensureTrailingNewline(mutated),
		DiffCode:    diff,
	}
}

// removeElseBlock creates a mutation that removes the else block.
func removeElseBlock(stmt *ast.IfStmt, fset *token.FileSet, content []byte, source m.Source) []m.Mutation {
	mutations := make([]m.Mutation, 0, 1)

	if stmt.Else == nil {
		return nil
	}

	// Find where the if block ends and else begins
	ifBlockEnd := stmt.Body.End()
	elseStart := stmt.Else.Pos()
	elseEnd := stmt.Else.End()

	ifEndOffset, ok1 := offsetForPos(fset, ifBlockEnd)
	elseStartOffset, ok2 := offsetForPos(fset, elseStart)
	elseEndOffset, ok3 := offsetForPos(fset, elseEnd)

	if !ok1 || !ok2 || !ok3 {
		return nil
	}

	// Find the "else" keyword position (between if block end and else block start)
	// We need to remove from after the if block's closing } to the end of else block
	elseKeywordStart := ifEndOffset
	for elseKeywordStart < len(content) && content[elseKeywordStart] != 'e' {
		elseKeywordStart++
	}

	mutated := replaceRange(content, elseKeywordStart, elseEndOffset, "")

	diff := diffCode(content, mutated)

	h := sha256.Sum256([]byte(fmt.Sprintf("%s-%s-%d", source.Origin.FullPath, m.MutationBranch.Name, elseStartOffset+2000)))
	id := fmt.Sprintf("%x", h)[:16]

	mutation := m.Mutation{
		ID:          id,
		Source:      source,
		Type:        m.MutationBranch,
		MutatedCode: ensureTrailingNewline(mutated),
		DiffCode:    diff,
	}

	mutations = append(mutations, mutation)

	return mutations
}

// mutateSwitchStatement creates mutations for switch statements and their cases.
func mutateSwitchStatement(stmt *ast.SwitchStmt, fset *token.FileSet, content []byte, source m.Source) []m.Mutation {
	var mutations []m.Mutation

	// Mutate switch tag if present
	if stmt.Tag != nil {
		mutations = append(mutations, mutateSwitchTag(stmt.Tag, fset, content, source)...)
	}

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
func mutateCaseClause(clause *ast.CaseClause, fset *token.FileSet, content []byte, source m.Source) []m.Mutation {
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
func removeCaseBody(clause *ast.CaseClause, fset *token.FileSet, content []byte, source m.Source) []m.Mutation {
	mutations := make([]m.Mutation, 0, 1)

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

	diff := diffCode(content, mutated)

	h := sha256.Sum256([]byte(fmt.Sprintf("%s-%s-%d", source.Origin.FullPath, m.MutationBranch.Name, colonOffset+3000)))
	id := fmt.Sprintf("%x", h)[:16]

	mutation := m.Mutation{
		ID:          id,
		Source:      source,
		Type:        m.MutationBranch,
		MutatedCode: ensureTrailingNewline(mutated),
		DiffCode:    diff,
	}

	mutations = append(mutations, mutation)

	return mutations
}

// invertCondition inverts a boolean condition expression.
func invertCondition(cond ast.Expr, fset *token.FileSet, content []byte, source m.Source) []m.Mutation {
	mutations := make([]m.Mutation, 0, 3)

	offset, ok1 := offsetForPos(fset, cond.Pos())

	endOffset, ok2 := offsetForPos(fset, cond.End())
	if !ok1 || !ok2 {
		return nil
	}

	originalExpr := string(content[offset:endOffset])

	// Create mutation that inverts the condition
	mutations = append(mutations, createConditionMutation(content, offset, endOffset, "!("+originalExpr+")", source, offset))

	// Force condition to true
	mutations = append(mutations, createConditionMutation(content, offset, endOffset, "true", source, offset+1))

	// Force condition to false
	mutations = append(mutations, createConditionMutation(content, offset, endOffset, "false", source, offset+2))

	return mutations
}

// createConditionMutation creates a single condition mutation.
func createConditionMutation(content []byte, offset, endOffset int, replacement string, source m.Source, idOffset int) m.Mutation {
	mutated := replaceRange(content, offset, endOffset, replacement)
	diff := diffCode(content, mutated)

	h := sha256.Sum256([]byte(fmt.Sprintf("%s-%s-%d", source.Origin.FullPath, m.MutationBranch.Name, idOffset)))
	id := fmt.Sprintf("%x", h)[:16]

	return m.Mutation{
		ID:          id,
		Source:      source,
		Type:        m.MutationBranch,
		MutatedCode: ensureTrailingNewline(mutated),
		DiffCode:    diff,
	}
}

// mutateSwitchTag creates mutations for switch statement tags.
func mutateSwitchTag(_ ast.Expr, _ *token.FileSet, _ []byte, _ m.Source) []m.Mutation {
	// For now, we'll skip switch tag mutations as they're complex
	// and require type analysis
	return nil
}
