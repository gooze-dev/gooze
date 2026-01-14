// Package domain provides core business logic for mutation testing operations.
package domain

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"

	m "github.com/mouse-blink/gooze/internal/model"
)

const (
	trueStr  = "true"
	falseStr = "false"
)

// GenerateMutations analyzes a source file and generates mutations based on type.
// If no types are specified, generates mutations for all supported types.
func (w *workflow) GenerateMutations(sources m.Source, mutationTypes ...m.MutationType) ([]m.Mutation, error) {
	// Default to all types if none specified
	if len(mutationTypes) == 0 {
		mutationTypes = []m.MutationType{m.MutationArithmetic, m.MutationBoolean}
	}

	// Validate mutation types
	for _, mutationType := range mutationTypes {
		if mutationType != m.MutationArithmetic && mutationType != m.MutationBoolean {
			return nil, fmt.Errorf("unsupported mutation type: %v", mutationType)
		}
	}

	source := sources
	// Parse the source file
	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, string(source.Origin), nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", source.Origin, err)
	}

	var mutations []m.Mutation

	mutationID := 0

	// Walk the AST and find mutations for each requested type
	for _, mutationType := range mutationTypes {
		ast.Inspect(file, func(n ast.Node) bool {
			switch mutationType {
			case m.MutationArithmetic:
				mutations = append(mutations, processArithmeticNode(n, fset, source, &mutationID)...)
			case m.MutationBoolean:
				mutations = append(mutations, processBooleanNode(n, fset, source, &mutationID)...)
			}

			return true
		})
	}

	return mutations, nil
}

// isArithmeticOp checks if a token is an arithmetic operator.
func isArithmeticOp(op token.Token) bool {
	return op == token.ADD || op == token.SUB || op == token.MUL || op == token.QUO || op == token.REM
}

// getArithmeticAlternatives returns all alternative operators for mutation.
func getArithmeticAlternatives(original token.Token) []token.Token {
	allOps := []token.Token{token.ADD, token.SUB, token.MUL, token.QUO, token.REM}

	var alternatives []token.Token

	for _, op := range allOps {
		if op != original {
			alternatives = append(alternatives, op)
		}
	}

	return alternatives
}

// findScopeType determines which scope a line belongs to.
func findScopeType(scopes []m.CodeScope, line int) m.ScopeType {
	for _, scope := range scopes {
		if line >= scope.StartLine && line <= scope.EndLine {
			return scope.Type
		}
	}

	// Default to function scope if not found in any scope
	return m.ScopeFunction
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

func processArithmeticNode(n ast.Node, fset *token.FileSet, source m.Source, mutationID *int) []m.Mutation {
	binExpr, ok := n.(*ast.BinaryExpr)
	if !ok {
		return nil
	}

	// Check if it's an arithmetic operator
	if !isArithmeticOp(binExpr.Op) {
		return nil
	}

	// Get position information
	pos := fset.Position(binExpr.OpPos)

	// Find which scope this mutation belongs to
	scopeType := findScopeType(source.Scopes, pos.Line)

	var mutations []m.Mutation
	// Generate mutations for all alternative operators
	for _, mutatedOp := range getArithmeticAlternatives(binExpr.Op) {
		*mutationID++
		mutations = append(mutations, m.Mutation{
			ID:         fmt.Sprintf("ARITH_%d", *mutationID),
			Type:       m.MutationArithmetic,
			SourceFile: source.Origin,
			OriginalOp: binExpr.Op,
			MutatedOp:  mutatedOp,
			Line:       pos.Line,
			Column:     pos.Column,
			ScopeType:  scopeType,
		})
	}

	return mutations
}

func processBooleanNode(n ast.Node, fset *token.FileSet, source m.Source, mutationID *int) []m.Mutation {
	ident, ok := n.(*ast.Ident)
	if !ok {
		return nil
	}

	// Check if it's a boolean literal
	if !isBooleanLiteral(ident.Name) {
		return nil
	}

	// Get position information
	pos := fset.Position(ident.Pos())

	// Find which scope this mutation belongs to
	scopeType := findScopeType(source.Scopes, pos.Line)

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
