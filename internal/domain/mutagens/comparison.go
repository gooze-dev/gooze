package mutagens

import (
	"go/ast"
	"go/parser"
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
		if skip := equivalentMutantOp(expr, fset, content); skip != token.ILLEGAL { //gooze:ignore comparison
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
func equivalentMutantOp(expr *ast.BinaryExpr, fset *token.FileSet, content []byte) token.Token {
	switch {
	case isMinimumOf(expr.Y, expr.X, fset, content):
		return equivalentOpWithMinimumRight(expr.Op)
	case isMinimumOf(expr.X, expr.Y, fset, content):
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
// the empty string, or 0 compared with a value that is never negative (see
// isNonNegative). v was parsed from content with fset.
func isMinimumOf(minimum, v ast.Expr, fset *token.FileSet, content []byte) bool {
	lit, ok := minimum.(*ast.BasicLit)
	if !ok {
		return false
	}

	switch lit.Kind {
	case token.STRING:
		return lit.Value == `""` || lit.Value == "``"
	case token.INT:
		// Equivalent mutants: no integer literal sorts before "0".
		return lit.Value == "0" && isNonNegative(v, fset, content) //gooze:ignore comparison
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

// isNonNegative reports whether e is never negative, judged from the syntax
// alone: a len or cap call, a variable declared with an unsigned type, or a
// variable defined from len or cap and never written again in content.
func isNonNegative(e ast.Expr, fset *token.FileSet, content []byte) bool {
	if isLenOrCapCall(e) {
		return true
	}

	ident, ok := e.(*ast.Ident)

	return ok && isNonNegativeVar(ident, fset, content)
}

// isNonNegativeVar reports whether ident is never negative: a variable or
// constant declared with an unsigned type, or a variable defined from len or
// cap and never written again.
func isNonNegativeVar(ident *ast.Ident, fset *token.FileSet, content []byte) bool {
	if ident.Obj == nil {
		return false
	}

	switch decl := ident.Obj.Decl.(type) {
	case *ast.Field:
		return isUnsignedType(decl.Type)
	case *ast.ValueSpec:
		return isUnsignedType(decl.Type)
	case *ast.AssignStmt:
		value := definedValue(decl, ident.Name)

		return value != nil && isLenOrCapCall(value) && !isWrittenAfterDefinition(fset, content, decl, ident.Name)
	default:
		return false
	}
}

// unsignedTypes are the predeclared integer types without negative values.
var unsignedTypes = map[string]bool{
	"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true, "uintptr": true, "byte": true,
}

// isUnsignedType reports whether t names a predeclared unsigned integer type;
// a type of the same name declared in the file is resolved to it and doesn't
// count.
func isUnsignedType(t ast.Expr) bool {
	ident, ok := t.(*ast.Ident)

	return ok && ident.Obj == nil && unsignedTypes[ident.Name]
}

// definedValue returns the value the := statement assign gives name, or nil
// when one call defines several names.
func definedValue(assign *ast.AssignStmt, name string) ast.Expr {
	// Equivalent: := never has more values than names, so != is >.
	if len(assign.Lhs) != len(assign.Rhs) { //gooze:ignore comparison
		return nil
	}

	for i, lhs := range assign.Lhs {
		if ident, ok := lhs.(*ast.Ident); ok && ident.Name == name {
			return assign.Rhs[i]
		}
	}

	return nil
}

// isWrittenAfterDefinition reports whether content writes the variable name
// that def defines anywhere but in def itself: assigned (also by a later :=
// or a range clause), incremented, decremented or its address taken. The file
// is parsed again because a mutagen only sees the node it mutates.
func isWrittenAfterDefinition(fset *token.FileSet, content []byte, def *ast.AssignStmt, name string) bool {
	defOffset := fset.Position(def.Pos()).Offset
	reparsed := token.NewFileSet()

	// content already parsed once, so this can't fail.
	file, _ := parser.ParseFile(reparsed, "", content, 0)

	// In the new tree, def is the := statement at the same offset.
	written := false

	ast.Inspect(file, func(n ast.Node) bool {
		for _, target := range writeTargets(n) {
			ident, ok := target.(*ast.Ident)
			if !ok || ident.Name != name || ident.Obj == nil {
				continue
			}

			decl, ok := ident.Obj.Decl.(*ast.AssignStmt)
			if ok && reparsed.Position(decl.Pos()).Offset == defOffset && n != ast.Node(decl) {
				written = true
			}
		}

		return !written
	})

	return written
}

// writeTargets returns the expressions n writes to. A range clause's key and
// value count even when it defines them: new variables never match def.
func writeTargets(n ast.Node) []ast.Expr {
	switch stmt := n.(type) {
	case *ast.AssignStmt:
		return stmt.Lhs
	case *ast.IncDecStmt:
		return []ast.Expr{stmt.X}
	case *ast.UnaryExpr:
		if stmt.Op == token.AND {
			return []ast.Expr{stmt.X}
		}
	case *ast.RangeStmt:
		return []ast.Expr{stmt.Key, stmt.Value}
	}

	return nil
}
