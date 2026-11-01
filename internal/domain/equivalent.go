package domain

import (
	"go/ast"
	"go/token"
	"slices"
	"strings"

	m "gooze.dev/pkg/gooze/internal/model"
)

// markEquivalentSubtrees adds to skip the expressions of file whose every
// mutant is equivalent, so the walk generates nothing inside them: the
// constant values returned next to a non-nil error or a false ok, which
// callers don't read (see markIgnoredResults), and the size hints of
// make(map[K]V, n), which only change allocation (see markMapSizeHint).
func markEquivalentSubtrees(file *ast.File, skip map[ast.Node]bool) {
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncDecl:
			if node.Body != nil {
				markIgnoredResults(node.Type, node.Body, skip)
			}
		case *ast.FuncLit:
			markIgnoredResults(node.Type, node.Body, skip)
		case *ast.CallExpr:
			markMapSizeHint(node, skip)
		}

		return true
	})
}

// markMapSizeHint marks the size hint of make(map[K]V, n): it only changes how
// much is allocated, and a negative hint doesn't panic. A slice's capacity is
// left alone, because a mutant that drops it below the length or 0 panics
// (makeslice: cap out of range), as are channel buffer sizes, which are
// behavior. The map type is recognized from the syntax, so a named map type
// keeps its mutants.
func markMapSizeHint(call *ast.CallExpr, skip map[ast.Node]bool) {
	if !isPredeclared(call.Fun, "make") {
		return
	}

	// A switch, not len(args) == 2, which would add the equivalent >= mutant
	// (make never takes three arguments for a map).
	switch args := call.Args; len(args) { //nolint:gocritic // see above
	case 2:
		if _, ok := args[0].(*ast.MapType); ok && isSafeHint(args[1]) {
			skip[args[1]] = true
		}
	}
}

// isSafeHint reports whether no mutant of the hint e can panic: e holds no
// function literal, whose body is code of its own, and each binary operator
// in it has a literal right operand, so swapping it for / or % can't divide
// by zero.
func isSafeHint(e ast.Expr) bool {
	safe := true

	ast.Inspect(e, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncLit:
			safe = false
		case *ast.BinaryExpr:
			if _, ok := node.Y.(*ast.BasicLit); !ok {
				safe = false
			}
		}

		return safe
	})

	return safe
}

// markIgnoredResults marks, in the return statements of one function body,
// the results returned next to a non-nil error or a literal false ok (see
// failureSignal): by Go convention callers don't read them. Only constant
// values are marked (see isConstantValue): a computed value may be a partial
// result that callers do read, as io.Reader's n is. Nested function literals
// are left to their own call.
func markIgnoredResults(fnType *ast.FuncType, body *ast.BlockStmt, skip map[ast.Node]bool) {
	results := resultTypes(fnType)
	if len(results) == 0 {
		return
	}

	last := len(results) - 1
	failure := failureSignal(results)

	ast.Inspect(body, func(n ast.Node) bool {
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}

		// A bare return or one call returning every value has nothing to mark.
		// Equivalent: a return never has more values than results, so >= is ==.
		ret, ok := n.(*ast.ReturnStmt)
		if ok && len(ret.Results) == len(results) && failure(ret.Results[last]) { //gooze:ignore comparison
			for _, value := range ret.Results[:last] {
				if isConstantValue(value) {
					skip[value] = true
				}
			}
		}

		return true
	})
}

// result is one result of a function: its type and its name, or "".
type result struct {
	typ  ast.Expr
	name string
}

// resultTypes returns each result of fnType, one per value.
func resultTypes(fnType *ast.FuncType) []result {
	if fnType.Results == nil {
		return nil
	}

	var results []result

	for _, field := range fnType.Results.List {
		if len(field.Names) == 0 {
			results = append(results, result{typ: field.Type})
		}

		for _, name := range field.Names {
			results = append(results, result{typ: field.Type, name: name.Name})
		}
	}

	return results
}

// failureSignal returns the test for the last returned value that tells
// callers to ignore the other results, or one that always fails: a non-nil
// error, or the literal false of a comma-ok bool. A bool counts as comma-ok
// only when it is unnamed or named ok and no other result is a bool, so
// results like (covered, known bool) keep their mutants. A type declared in
// the file under either name is resolved to it and doesn't count.
func failureSignal(results []result) func(value ast.Expr) bool {
	last := results[len(results)-1]

	switch {
	case isPredeclared(last.typ, "error"):
		return func(value ast.Expr) bool { return !isPredeclared(value, "nil") }
	case isCommaOk(results):
		return func(value ast.Expr) bool { return isPredeclared(value, "false") }
	default:
		return func(ast.Expr) bool { return false }
	}
}

// isCommaOk reports whether the last of results is a comma-ok bool: a bool,
// unnamed or named ok, after results none of which is a bool.
func isCommaOk(results []result) bool {
	last := results[len(results)-1]
	if !isPredeclared(last.typ, "bool") || last.name != "" && last.name != "ok" {
		return false
	}

	for _, r := range results[:len(results)-1] {
		if isPredeclared(r.typ, "bool") {
			return false
		}
	}

	return true
}

// isPredeclared reports whether e is the predeclared identifier name, not a
// declaration of the same name in the file.
func isPredeclared(e ast.Expr, name string) bool {
	ident, ok := e.(*ast.Ident)

	return ok && ident.Name == name && ident.Obj == nil
}

// isConstantValue reports whether e is a literal value: a basic literal, true,
// false or nil, a signed basic literal, or a composite literal of such values,
// possibly with its address taken.
func isConstantValue(e ast.Expr) bool {
	switch v := e.(type) {
	case *ast.BasicLit:
		return true
	case *ast.Ident:
		return isPredeclared(v, "true") || isPredeclared(v, "false") || isPredeclared(v, "nil")
	case *ast.UnaryExpr:
		return isConstantUnary(v)
	case *ast.CompositeLit:
		return isConstantComposite(v)
	case *ast.ParenExpr:
		return isConstantValue(v.X)
	default:
		return false
	}
}

// isConstantUnary reports whether e is a signed constant value (-1, +2) or
// the address of one (&T{}); Go rejects these operators on other constant
// values.
func isConstantUnary(e *ast.UnaryExpr) bool {
	switch e.Op {
	case token.SUB, token.ADD, token.AND:
		return isConstantValue(e.X)
	default:
		return false
	}
}

// isConstantComposite reports whether every element of the composite literal
// e, or the value of every key: value pair, is a constant value.
func isConstantComposite(e *ast.CompositeLit) bool {
	for _, elt := range e.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			elt = kv.Value
		}

		if !isConstantValue(elt) {
			return false
		}
	}

	return true
}

// strconvBitSizeArg is the position of the bitSize argument of the strconv
// functions that accept an equivalent size. FormatFloat isn't one: it panics
// on any size but 32 and 64.
var strconvBitSizeArg = map[string]int{"ParseInt": 2, "ParseUint": 2, "ParseFloat": 1}

// halfRotations holds half the size of each math/bits rotation with a fixed
// size. RotateLeft isn't one: its size depends on the host.
var halfRotations = map[string]string{"RotateLeft8": "4", "RotateLeft16": "8", "RotateLeft32": "16", "RotateLeft64": "32"}

// markPartlyEquivalent finds the nodes with an equivalent mutant next to ones
// that can be killed, so skip can't hold them whole.
//
// In the literal bitSize 64 of a strconv call, ParseFloat treats every size but
// 32 as 64, so its literal goes in skip. ParseInt and ParseUint read 0 as the
// size of int, which is 64 on the 64-bit hosts gooze mostly runs on, so only
// the mutant to 0 is equivalent: the literal is in the returned set.
//
// Rotating by half a value's size gives the same result in either direction,
// so the sign of such an amount is in the returned set; the literal inside
// keeps its mutants. See dropEquivalent.
func markPartlyEquivalent(file *ast.File, skip map[ast.Node]bool) map[ast.Node]bool {
	partly := map[ast.Node]bool{}
	strconvName, bitsName := importName(file, "strconv"), importName(file, "math/bits")

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		if sign := halfRotationSign(call, bitsName); sign != nil {
			partly[sign] = true
		}

		function, lit := strconvBitSize64(call, strconvName)

		switch {
		case lit == nil:
		// Equivalent: ParseFloat sorts first of the functions strconvBitSize64 returns.
		case function == "ParseFloat": //gooze:ignore comparison
			skip[lit] = true
		default:
			partly[lit] = true
		}

		return true
	})

	return partly
}

// halfRotationSign returns the signed amount of call when it rotates by half
// the size of the value, with a sign on the literal: bits.RotateLeft64(x, -32).
func halfRotationSign(call *ast.CallExpr, pkg string) *ast.UnaryExpr {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !isPredeclared(selector.X, pkg) || len(call.Args) != 2 {
		return nil
	}

	sign, ok := call.Args[1].(*ast.UnaryExpr)
	// Equivalent: + and - are the lowest unary operators, so > is != for them.
	if !ok || (sign.Op != token.SUB && sign.Op != token.ADD) { //gooze:ignore comparison
		return nil
	}

	lit, ok := sign.X.(*ast.BasicLit)
	if !ok || lit.Value != halfRotations[selector.Sel.Name] {
		return nil
	}

	return sign
}

// strconvBitSize64 returns the function called and its bitSize argument, when
// call is a strconv call in strconvBitSizeArg whose bitSize is the literal 64.
func strconvBitSize64(call *ast.CallExpr, pkg string) (string, *ast.BasicLit) {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !isPredeclared(selector.X, pkg) {
		return "", nil
	}

	arg, ok := strconvBitSizeArg[selector.Sel.Name]
	if !ok || len(call.Args) != arg+1 {
		return "", nil
	}

	lit, ok := call.Args[arg].(*ast.BasicLit)
	if !ok || lit.Value != "64" {
		return "", nil
	}

	return selector.Sel.Name, lit
}

// importName returns the name file imports path under, or "".
func importName(file *ast.File, path string) string {
	for _, spec := range file.Imports {
		if spec.Path.Value != `"`+path+`"` {
			continue
		}

		if spec.Name != nil {
			return spec.Name.Name
		}

		return path[strings.LastIndex(path, "/")+1:]
	}

	return ""
}

// dropEquivalent removes the equivalent mutants of n when n is in partly (see
// markPartlyEquivalent): every mutant of a sign, and the one that turns a
// number into 0.
func dropEquivalent(mutations []m.Mutation, n ast.Node, fset *token.FileSet, partly map[ast.Node]bool) []m.Mutation {
	if !partly[n] {
		return mutations
	}

	if _, isSign := n.(*ast.UnaryExpr); isSign {
		return nil
	}

	offset := fset.Position(n.Pos()).Offset

	return slices.DeleteFunc(mutations, func(mutation m.Mutation) bool {
		// Equivalent: a number's mutants start with 0 or 1, so <= '0' is == '0'.
		return mutation.MutatedCode[offset] == '0' //gooze:ignore comparison
	})
}
