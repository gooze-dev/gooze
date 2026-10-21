package mutagens

import (
	"go/ast"
	"go/token"

	m "gooze.dev/pkg/gooze/internal/model"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// bitwiseAlternatives maps each bitwise operator to the one it becomes. One
// mutant per operator keeps the noise of near-equivalent swaps down.
var bitwiseAlternatives = map[token.Token]token.Token{
	token.AND:     token.OR,  // & -> |
	token.OR:      token.AND, // | -> &
	token.XOR:     token.AND, // ^ -> &
	token.AND_NOT: token.AND, // &^ -> &
	token.SHL:     token.SHR, // << -> >>
	token.SHR:     token.SHL, // >> -> <<
}

// GenerateBitwiseMutations generates bitwise mutations for the given AST node.
func GenerateBitwiseMutations(n ast.Node, fset *token.FileSet, content []byte, source rpt.Source) []m.Mutation {
	return generateBinaryExprMutations(n, fset, content, source, mgen.Bitwise, isBitwiseOp, getBitwiseAlternatives)
}

func isBitwiseOp(op token.Token) bool {
	_, ok := bitwiseAlternatives[op]

	return ok
}

func getBitwiseAlternatives(original token.Token) []token.Token {
	return []token.Token{bitwiseAlternatives[original]}
}
