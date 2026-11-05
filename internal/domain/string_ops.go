package domain

import (
	"bytes"
	"context"
	"go/ast"
	"go/token"
	"slices"

	"gooze.dev/pkg/gooze/internal/domain/mutagens"
	m "gooze.dev/pkg/gooze/internal/model"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// stringExprs returns the spans of the string expressions of source when a
// mutagen in types can put an operator on a string that doesn't build, and
// the mutagen has type information; otherwise nil.
func (mg *mutagen) stringExprs(ctx context.Context, source rpt.Source, types []mgen.Type) map[Span]bool {
	if mg.types == nil || (!slices.Contains(types, mgen.Arithmetic) && !slices.Contains(types, mgen.Assignment)) {
		return nil
	}

	return mg.types.StringExprs(ctx, source.Origin.FullPath)
}

// integerExprs returns the spans of the integer expressions of source when a
// mutagen in types can make a divisor zero (see buildChecks) and the mutagen
// has type information; otherwise nil.
func (mg *mutagen) integerExprs(ctx context.Context, source rpt.Source, types []mgen.Type) map[Span]bool {
	if mg.types == nil || (!slices.Contains(types, mgen.Numbers) && !slices.Contains(types, mgen.Arithmetic)) {
		return nil
	}

	return mg.types.IntegerExprs(ctx, source.Origin.FullPath)
}

// unorderedExprs returns the spans of the expressions of source whose type
// has no order when types ask for comparison mutants and the mutagen has
// type information; otherwise nil.
func (mg *mutagen) unorderedExprs(ctx context.Context, source rpt.Source, types []mgen.Type) map[Span]bool {
	if mg.types == nil || !slices.Contains(types, mgen.Comparison) {
		return nil
	}

	return mg.types.UnorderedExprs(ctx, source.Origin.FullPath)
}

// dropStringOps drops the mutants of n that can't build because they put an
// operator other than + on strings (strs, nil when unknown): every arithmetic
// mutant of a string-typed binary expression, whose operator can only be +,
// and the -= mutant of each s += x in a block whose s is a string. The =
// mutant of s += x builds and is kept.
func dropStringOps(mutationType mgen.Type, mutations []m.Mutation, n ast.Node, fset *token.FileSet, content []byte, strs map[Span]bool) []m.Mutation {
	isString := func(node ast.Node) bool {
		return strs[Span{Start: fset.Position(node.Pos()).Offset, End: fset.Position(node.End()).Offset}]
	}

	switch mutationType {
	case mgen.Arithmetic:
		if _, ok := n.(*ast.BinaryExpr); ok && isString(n) {
			return nil
		}
	case mgen.Assignment:
		return dropStringSubAssign(mutations, n, fset, content, isString)
	}

	return mutations
}

// dropStringSubAssign drops the -= mutant of each s += x among the statements
// of block n whose s is a string, the statements the assignment mutagen
// mutates. Only += exists on strings, so any assignment to a string can be
// checked: the others have no -= mutant.
func dropStringSubAssign(mutations []m.Mutation, n ast.Node, fset *token.FileSet, content []byte, isString func(ast.Node) bool) []m.Mutation {
	for _, stmt := range mutagens.BlockStatements(n) {
		assign, ok := stmt.(*ast.AssignStmt)
		if !ok || !isString(assign.Lhs[0]) {
			continue
		}

		at := fset.Position(assign.TokPos).Offset
		minus := slices.Concat(content[:at], []byte("-="), content[at+len("+="):])
		mutations = slices.DeleteFunc(mutations, func(mutation m.Mutation) bool {
			return bytes.Equal(mutation.MutatedCode, minus)
		})
	}

	return mutations
}
