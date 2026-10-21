package mutagens

import (
	"go/ast"
	"go/token"

	m "gooze.dev/pkg/gooze/internal/model"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// assignmentAlternatives maps each compound assignment to the one it becomes,
// swapped as its operator is by the arithmetic and bitwise mutagens.
var assignmentAlternatives = map[token.Token]token.Token{
	token.ADD_ASSIGN:     token.SUB_ASSIGN, // += -> -=
	token.SUB_ASSIGN:     token.ADD_ASSIGN, // -= -> +=
	token.MUL_ASSIGN:     token.QUO_ASSIGN, // *= -> /=
	token.QUO_ASSIGN:     token.MUL_ASSIGN, // /= -> *=
	token.REM_ASSIGN:     token.MUL_ASSIGN, // %= -> *=
	token.AND_ASSIGN:     token.OR_ASSIGN,  // &= -> |=
	token.OR_ASSIGN:      token.AND_ASSIGN, // |= -> &=
	token.XOR_ASSIGN:     token.AND_ASSIGN, // ^= -> &=
	token.AND_NOT_ASSIGN: token.AND_ASSIGN, // &^= -> &=
	token.SHL_ASSIGN:     token.SHR_ASSIGN, // <<= -> >>=
	token.SHR_ASSIGN:     token.SHL_ASSIGN, // >>= -> <<=
}

// GenerateAssignmentMutations mutates the compound assignments among the
// statements of the given block: x op= y gets the swapped operator and
// becomes x = y. Like the increment mutagen, it leaves a for loop's post
// statement alone.
func GenerateAssignmentMutations(n ast.Node, fset *token.FileSet, content []byte, source rpt.Source) []m.Mutation {
	var mutations []m.Mutation

	for _, stmt := range blockStatements(n) {
		assign, ok := stmt.(*ast.AssignStmt)
		if !ok {
			continue
		}

		swapped, ok := assignmentAlternatives[assign.Tok]
		if !ok {
			continue
		}

		replacements := []token.Token{swapped, token.ASSIGN}
		if len(assign.Rhs) == 1 && isStringLit(assign.Rhs[0]) {
			// Strings support only +=.
			replacements = []token.Token{token.ASSIGN}
		}

		mutations = append(mutations, replaceToken(fset, content, source, mgen.Assignment, assign.TokPos, assign.Tok, replacements...)...)
	}

	return mutations
}
