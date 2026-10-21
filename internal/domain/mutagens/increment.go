package mutagens

import (
	"go/ast"
	"go/token"

	m "gooze.dev/pkg/gooze/internal/model"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// incDecSwaps maps each of ++ and -- to the other.
var incDecSwaps = map[token.Token]token.Token{token.INC: token.DEC, token.DEC: token.INC}

// GenerateIncrementMutations swaps x++ and x-- in the statements of the given
// block. Only statements in a list are mutated, so a for loop's post
// statement, where i-- would loop until the test times out, is left alone.
func GenerateIncrementMutations(n ast.Node, fset *token.FileSet, content []byte, source rpt.Source) []m.Mutation {
	var mutations []m.Mutation

	for _, stmt := range blockStatements(n) {
		incDec, ok := stmt.(*ast.IncDecStmt)
		if !ok {
			continue
		}

		mutations = append(mutations, replaceToken(fset, content, source, mgen.Increment, incDec.TokPos, incDec.Tok, incDecSwaps[incDec.Tok])...)
	}

	return mutations
}

// blockStatements returns the statement list of a block, case or select
// clause, or nil for any other node.
func blockStatements(n ast.Node) []ast.Stmt {
	switch block := n.(type) {
	case *ast.BlockStmt:
		return block.List
	case *ast.CaseClause:
		return block.Body
	case *ast.CommClause:
		return block.Body
	}

	return nil
}

// replaceToken returns the mutations that replace the token original at pos
// with each of replacements.
func replaceToken(
	fset *token.FileSet,
	content []byte,
	source rpt.Source,
	mutationType mgen.Type,
	pos token.Pos,
	original token.Token,
	replacements ...token.Token,
) []m.Mutation {
	start, ok := offsetForPos(fset, pos)
	if !ok {
		return nil
	}

	mutations := make([]m.Mutation, 0, len(replacements))

	for _, replacement := range replacements {
		mutatedCode := replaceRange(content, start, start+len(original.String()), replacement.String())
		mutations = append(mutations, m.Mutation{
			ID:          mutationID(mutatedCode),
			Source:      source,
			Type:        mutationType,
			MutatedCode: mutatedCode,
		})
	}

	return mutations
}
