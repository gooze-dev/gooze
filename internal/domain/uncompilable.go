package domain

import (
	"go/ast"
	"go/constant"
	"go/parser"
	"go/token"
	"log/slog"
	"slices"

	m "gooze.dev/pkg/gooze/internal/model"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
)

// buildChecks finds the mutants of a file that can't build, so they aren't
// tested: an operator other than + on strings (see dropStringOps), a
// constant divisor of an integer division made zero, a case value made equal
// to another constant case of its switch, and an ordering comparison of
// operands without an order. It counts the mutants it skips by mutagen.
type buildChecks struct {
	typed typeInfo
	fset  *token.FileSet
	// divisors are the divisors of the integer divisions and remainders.
	divisors []Span
	// cases are the case values of each switch with a tag.
	cases [][]Span
	// unordered are the offsets of the operators of the binary expressions
	// with an operand whose type has no order: no ordering operator builds
	// there.
	unordered map[int]bool
	skipped   map[string]int
}

// newBuildChecks indexes the divisions, switches and comparisons of file.
// Without type information (typed is empty) only the divisions of constants
// are known to be integer ones, and no operand is known to have no order.
func newBuildChecks(file *ast.File, fset *token.FileSet, typed typeInfo) *buildChecks {
	checks := &buildChecks{typed: typed, fset: fset, unordered: map[int]bool{}, skipped: map[string]int{}}

	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.BinaryExpr:
			checks.indexBinary(node)
		case *ast.AssignStmt:
			if (node.Tok == token.QUO_ASSIGN || node.Tok == token.REM_ASSIGN) && checks.isInteger(node.Lhs[0]) {
				checks.divisors = append(checks.divisors, checks.spanOf(node.Rhs[0]))
			}
		case *ast.SwitchStmt:
			if node.Tag != nil {
				checks.cases = append(checks.cases, caseValues(node, checks.spanOf))
			}
		}

		return true
	})

	return checks
}

// indexBinary records the divisor of an integer division or remainder, and
// the operator of a binary expression with an operand without an order.
func (c *buildChecks) indexBinary(node *ast.BinaryExpr) {
	if (node.Op == token.QUO || node.Op == token.REM) && c.isInteger(node.X) {
		c.divisors = append(c.divisors, c.spanOf(node.Y))
	}

	if c.typed.unordered[c.spanOf(node.X)] || c.typed.unordered[c.spanOf(node.Y)] {
		c.unordered[c.fset.Position(node.OpPos).Offset] = true
	}
}

// spanOf returns the span of n.
func (c *buildChecks) spanOf(n ast.Node) Span {
	return Span{Start: c.fset.Position(n.Pos()).Offset, End: c.fset.Position(n.End()).Offset}
}

// isInteger reports whether e is known to be an integer: type information
// says so, or e is a constant, whose division by zero doesn't build either.
func (c *buildChecks) isInteger(e ast.Expr) bool {
	_, known := constValue(e)

	return known || c.typed.integers[c.spanOf(e)]
}

// caseValues returns the spans of the case values of sw.
func caseValues(sw *ast.SwitchStmt, spanOf func(ast.Node) Span) []Span {
	var spans []Span

	for _, stmt := range sw.Body.List {
		if clause, ok := stmt.(*ast.CaseClause); ok {
			for _, value := range clause.List {
				spans = append(spans, spanOf(value))
			}
		}
	}

	return spans
}

// keepBuilding drops the mutants of n that can't build and counts them.
func (c *buildChecks) keepBuilding(mutationType mgen.Type, mutations []m.Mutation, n ast.Node, fset *token.FileSet, content []byte) []m.Mutation {
	before := len(mutations)
	mutations = dropStringOps(mutationType, mutations, n, fset, content, c.typed.strings)
	mutations = slices.DeleteFunc(mutations, func(mutation m.Mutation) bool {
		return c.breaksBuild(content, mutation.MutatedCode)
	})
	c.skipped[mutationType.Name] += before - len(mutations)

	return mutations
}

// breaksBuild reports whether mutated, a change of content, makes a divisor
// constant zero, a case value equal to another of its switch, or orders
// operands without an order.
func (c *buildChecks) breaksBuild(content, mutated []byte) bool {
	at := firstDifference(content, mutated)
	if at < 0 {
		return false
	}

	if c.unordered[at] && slices.Contains([]string{"<", "<=", ">", ">="}, replacementOf(content, mutated)) {
		return true
	}

	for _, divisor := range c.divisors {
		if divisor.contains(at) && isZero(mutatedValue(divisor, content, mutated)) {
			return true
		}
	}

	return slices.ContainsFunc(c.cases, func(values []Span) bool {
		return duplicatesCase(values, at, content, mutated)
	})
}

// duplicatesCase reports whether mutated, changed at offset at, gives one of
// the case values the value of another.
func duplicatesCase(values []Span, at int, content, mutated []byte) bool {
	i := slices.IndexFunc(values, func(value Span) bool { return value.contains(at) })
	if i < 0 {
		return false
	}

	changed, known := mutatedValue(values[i], content, mutated)
	if !known {
		return false
	}

	for j, value := range values {
		if other, ok := exprValue(content[value.Start:value.End]); ok && j != i && equalValues(changed, other) {
			return true
		}
	}

	return false
}

// contains reports whether offset is inside s.
func (s Span) contains(offset int) bool {
	return s.Start <= offset && offset < s.End
}

// mutatedValue returns the constant value of the expression at s of content
// once mutated, which changes it, has changed it, and whether it is known.
func mutatedValue(s Span, content, mutated []byte) (constant.Value, bool) {
	return exprValue(mutated[s.Start : s.End+len(mutated)-len(content)])
}

// exprValue returns the constant value of the Go expression src, and whether
// it is known: src only has literals and operators.
func exprValue(src []byte) (constant.Value, bool) {
	expr, err := parser.ParseExpr(string(src))
	if err != nil {
		return nil, false
	}

	return constValue(expr)
}

// isZero reports whether a known value is zero.
func isZero(v constant.Value, known bool) bool {
	return known && constant.Sign(v) == 0
}

// equalValues reports whether a and b are equal: two numbers or two strings
// with the same value.
func equalValues(a, b constant.Value) bool {
	sameKind := isNumber(a) && isNumber(b) || isString(a) && isString(b)

	return sameKind && constant.Compare(a, token.EQL, b)
}

// kindIs reports whether v is of one of kinds.
func kindIs(v constant.Value, kinds ...constant.Kind) bool {
	return slices.Contains(kinds, v.Kind())
}

func isNumber(v constant.Value) bool {
	return kindIs(v, constant.Int, constant.Float, constant.Complex)
}
func isInt(v constant.Value) bool    { return kindIs(v, constant.Int) }
func isString(v constant.Value) bool { return kindIs(v, constant.String) }

func bothNumbers(x, y constant.Value) bool { return isNumber(x) && isNumber(y) }
func bothInts(x, y constant.Value) bool    { return isInt(x) && isInt(y) }

// constValue returns the value of e and whether it is known: e is built of
// literals, parentheses and arithmetic operators. A name, a call, a shift or
// a division by zero is unknown.
func constValue(e ast.Expr) (constant.Value, bool) {
	switch node := e.(type) {
	case *ast.BasicLit:
		// A literal of a file that parsed is valid.
		return constant.MakeFromLiteral(node.Value, node.Kind, 0), true
	case *ast.ParenExpr:
		return constValue(node.X)
	case *ast.UnaryExpr:
		return unaryValue(node)
	case *ast.BinaryExpr:
		return binaryValue(node)
	default:
		return nil, false
	}
}

// unaryOperands are the unary operators constValue knows, with the operands
// each takes.
var unaryOperands = map[token.Token]func(constant.Value) bool{
	token.ADD: isNumber,
	token.SUB: isNumber,
	token.XOR: isInt,
}

// unaryValue is constValue for +x, -x and ^x.
func unaryValue(node *ast.UnaryExpr) (constant.Value, bool) {
	x, ok := constValue(node.X)
	takes, known := unaryOperands[node.Op]

	if !ok || !known || !takes(x) {
		return nil, false
	}

	return constant.UnaryOp(node.Op, x, 0), true
}

// binaryOperands are the binary operators constValue knows, with the
// operands each takes.
var binaryOperands = map[token.Token]func(x, y constant.Value) bool{
	token.ADD:     func(x, y constant.Value) bool { return bothNumbers(x, y) || isString(x) && isString(y) },
	token.SUB:     bothNumbers,
	token.MUL:     bothNumbers,
	token.QUO:     bothNumbers,
	token.REM:     bothInts,
	token.AND:     bothInts,
	token.OR:      bothInts,
	token.XOR:     bothInts,
	token.AND_NOT: bothInts,
}

// binaryValue is constValue for the arithmetic operators. Dividing two
// integers truncates, as Go does; dividing by zero is unknown.
func binaryValue(node *ast.BinaryExpr) (constant.Value, bool) {
	x, okX := constValue(node.X)
	y, okY := constValue(node.Y)
	takes, known := binaryOperands[node.Op]

	switch {
	case !okX || !okY || !known || !takes(x, y):
		return nil, false
	case slices.Contains([]token.Token{token.QUO, token.REM}, node.Op) && constant.Sign(y) == 0:
		return nil, false
	case node.Op == token.QUO && bothInts(x, y):
		return constant.BinaryOp(x, token.QUO_ASSIGN, y), true
	default:
		return constant.BinaryOp(x, node.Op, y), true
	}
}

// logSkipped logs, at debug level, how many mutants of file each mutagen
// skipped because they can't build.
func (c *buildChecks) logSkipped(file string) {
	for name, count := range c.skipped {
		// Equivalent mutant: a count is never negative.
		if count > 0 { //gooze:ignore comparison
			slog.Debug("Skipped mutants that can't build", "file", file, "mutagen", name, "count", count)
		}
	}
}
