package mutagens

import (
	"go/ast"
	"go/token"
	"slices"

	m "gooze.dev/pkg/gooze/internal/model"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// GenerateComparisonMutations generates comparison operator mutations for the
// given AST node, leaving out the mutant that is equivalent because one operand
// is the minimum of the other (see equivalentMutantOp).
func GenerateComparisonMutations(n ast.Node, fset *token.FileSet, content []byte, source rpt.Source) []m.Mutation {
	alternatives := getComparisonAlternatives

	if expr, ok := n.(*ast.BinaryExpr); ok {
		// Equivalent mutants: token.ILLEGAL is the lowest token.
		if skip := equivalentMutantOp(expr); skip != token.ILLEGAL { //gooze:ignore comparison
			alternatives = func(op token.Token) []token.Token {
				return slices.DeleteFunc(getComparisonAlternatives(op), func(alt token.Token) bool { return alt == skip })
			}
		}
	}

	return generateBinaryExprMutations(n, fset, content, source, mgen.Comparison, isComparisonOp, alternatives)
}

func isComparisonOp(op token.Token) bool {
	return op == token.LSS || op == token.GTR || op == token.LEQ ||
		op == token.GEQ || op == token.EQL || op == token.NEQ
}

func getComparisonAlternatives(original token.Token) []token.Token {
	allOps := []token.Token{token.LSS, token.GTR, token.LEQ, token.GEQ, token.EQL, token.NEQ}

	var alternatives []token.Token

	for _, op := range allOps {
		if op != original {
			alternatives = append(alternatives, op)
		}
	}

	return alternatives
}

// equivalentMutantOp returns the operator whose mutant of expr is equivalent to
// expr because one operand is a minimum m of the other operand v, or
// token.ILLEGAL. Nothing is below m, so "v == m" behaves as "v <= m",
// "v != m" as "v > m", and the other way around; mirrored when m is on the
// left.
func equivalentMutantOp(expr *ast.BinaryExpr) token.Token {
	switch {
	case isMinimumOf(expr.Y, expr.X):
		return equivalentOpWithMinimumRight(expr.Op)
	case isMinimumOf(expr.X, expr.Y):
		return equivalentOpWithMinimumLeft(expr.Op)
	default:
		return token.ILLEGAL
	}
}

func equivalentOpWithMinimumRight(op token.Token) token.Token {
	switch op {
	case token.EQL:
		return token.LEQ
	case token.NEQ:
		return token.GTR
	case token.GTR:
		return token.NEQ
	case token.LEQ:
		return token.EQL
	default:
		return token.ILLEGAL
	}
}

func equivalentOpWithMinimumLeft(op token.Token) token.Token {
	switch op {
	case token.EQL:
		return token.GEQ
	case token.NEQ:
		return token.LSS
	case token.LSS:
		return token.NEQ
	case token.GEQ:
		return token.EQL
	default:
		return token.ILLEGAL
	}
}

// isMinimumOf reports whether minimum is a literal no value of v can be below:
// the empty string, or 0 compared with a len or cap call. Without type
// information, unsigned integers are not recognized.
func isMinimumOf(minimum, v ast.Expr) bool {
	lit, ok := minimum.(*ast.BasicLit)
	if !ok {
		return false
	}

	switch lit.Kind {
	case token.STRING:
		return lit.Value == `""` || lit.Value == "``"
	case token.INT:
		// Equivalent mutants: no integer literal sorts before "0".
		return lit.Value == "0" && isLenOrCapCall(v) //gooze:ignore comparison
	default:
		return false
	}
}

func isLenOrCapCall(e ast.Expr) bool {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return false
	}

	ident, ok := call.Fun.(*ast.Ident)

	return ok && (ident.Name == "len" || ident.Name == "cap")
}
