package domain

import (
	"go/ast"
	"go/token"
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
