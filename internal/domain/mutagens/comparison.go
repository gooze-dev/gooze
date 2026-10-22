package mutagens

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strconv"
	"strings"
	"sync"

	m "gooze.dev/pkg/gooze/internal/model"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// GenerateComparisonMutations generates comparison operator mutations for the
// given AST node, leaving out the mutants that are equivalent because one
// operand is the minimum of the other (see equivalentMutantOp) or because the
// comparison stops a counting loop (see countingLoopMutantOp).
func GenerateComparisonMutations(n ast.Node, fset *token.FileSet, content []byte, source rpt.Source) []m.Mutation {
	alternatives := getComparisonAlternatives

	if expr, ok := n.(*ast.BinaryExpr); ok && isComparisonOp(expr.Op) {
		skip := []token.Token{equivalentMutantOp(expr, fset, content), countingLoopMutantOp(expr, fset, content)}
		alternatives = func(op token.Token) []token.Token {
			return slices.DeleteFunc(getComparisonAlternatives(op), func(alt token.Token) bool { return slices.Contains(skip, alt) })
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
// alone: a len or cap call, a call the standard library documents as never
// negative (see nonNegativeFuncs and nonNegativeMethods), or a variable that
// can't be negative (see isNonNegativeVar).
func isNonNegative(e ast.Expr, fset *token.FileSet, content []byte) bool {
	switch v := e.(type) {
	case *ast.CallExpr:
		return isLenOrCapCall(v) || isNonNegativeCall(v, content)
	case *ast.Ident:
		return isNonNegativeVar(v, fset, content)
	default:
		return false
	}
}

// Standard-library names used in the tables below.
const (
	pkgBytes   = "bytes"
	pkgFlag    = "flag"
	pkgStrings = "strings"

	typeBuffer  = pkgBytes + ".Buffer"
	typeFlagSet = pkgFlag + ".FlagSet"

	methodLen = "Len"
	methodCap = "Cap"
)

// nonNegativeFuncs are the standard-library functions, by import path and
// name, that never return a negative number.
var nonNegativeFuncs = map[[2]string]bool{
	{pkgStrings, "Count"}:                 true,
	{pkgBytes, "Count"}:                   true,
	{"unicode/utf8", "RuneCount"}:         true,
	{"unicode/utf8", "RuneCountInString"}: true,
	{pkgFlag, "NArg"}:                     true,
	{pkgFlag, "NFlag"}:                    true,
	{"runtime", "NumGoroutine"}:           true,
	{"runtime", "NumCPU"}:                 true,
}

// nonNegativeMethods are the standard-library methods, by receiver type and
// name, that never return a negative number.
var nonNegativeMethods = map[[2]string]bool{
	{pkgStrings + ".Builder", methodLen}: true,
	{pkgStrings + ".Builder", methodCap}: true,
	{pkgStrings + ".Reader", methodLen}:  true,
	{typeBuffer, methodLen}:              true,
	{typeBuffer, methodCap}:              true,
	{pkgBytes + ".Reader", methodLen}:    true,
	{typeFlagSet, "NArg"}:                true,
	{typeFlagSet, "NFlag"}:               true,
}

// constructors are the standard-library functions, by import path and name,
// that return a value of a type in nonNegativeMethods.
var constructors = map[[2]string]string{
	{pkgStrings, "NewReader"}:     pkgStrings + ".Reader",
	{pkgBytes, "NewReader"}:       pkgBytes + ".Reader",
	{pkgBytes, "NewBuffer"}:       typeBuffer,
	{pkgBytes, "NewBufferString"}: typeBuffer,
	{pkgFlag, "NewFlagSet"}:       typeFlagSet,
}

// isNonNegativeCall reports whether call is a function in nonNegativeFuncs or
// a method in nonNegativeMethods on a variable whose declared type is visible
// in the file (see declaredType); a field's type isn't looked up.
func isNonNegativeCall(call *ast.CallExpr, content []byte) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	x, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}

	imports := fileImports(content)

	// A package name isn't resolved to a declaration in the file.
	if x.Obj == nil {
		return nonNegativeFuncs[[2]string{imports[x.Name], sel.Sel.Name}]
	}

	return nonNegativeMethods[[2]string{declaredType(x, imports), sel.Sel.Name}]
}

// declaredType returns the standard-library type of the variable ident, as
// "pkg.Type", from its declaration: a parameter or var with that type (or a
// pointer to it), or a variable defined from a composite literal or a
// function in constructors. It returns "" when the type isn't known.
func declaredType(ident *ast.Ident, imports map[string]string) string {
	switch decl := ident.Obj.Decl.(type) {
	case *ast.Field:
		return qualifiedType(decl.Type, imports)
	case *ast.ValueSpec:
		return qualifiedType(decl.Type, imports)
	case *ast.AssignStmt:
		return valueType(definedValue(decl, ident.Name), imports)
	default:
		return ""
	}
}

// valueType returns the standard-library type of value, as "pkg.Type": a
// composite literal (or its address), or a call to a function in
// constructors. It returns "" when the type isn't known.
func valueType(value ast.Expr, imports map[string]string) string {
	switch v := value.(type) {
	case *ast.UnaryExpr:
		return valueType(v.X, imports)
	case *ast.CompositeLit:
		return qualifiedType(v.Type, imports)
	case *ast.CallExpr:
		if sel, ok := v.Fun.(*ast.SelectorExpr); ok {
			if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Obj == nil {
				return constructors[[2]string{imports[pkg.Name], sel.Sel.Name}]
			}
		}
	}

	return ""
}

// qualifiedType returns t as "importpath.Type" for a type from an imported
// package, or a pointer to one, and "" for any other type. In a type, the
// part before the dot is always a package name.
func qualifiedType(t ast.Expr, imports map[string]string) string {
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}

	sel, ok := t.(*ast.SelectorExpr)
	if !ok {
		return ""
	}

	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return ""
	}

	return imports[pkg.Name] + "." + sel.Sel.Name
}

// fileImports returns the import path of each package name content imports;
// a renamed import is listed under its new name, and a blank or dot import
// under none.
func fileImports(content []byte) map[string]string {
	file, _ := reparse(content)
	imports := map[string]string{}

	for _, spec := range file.Imports {
		path, _ := strconv.Unquote(spec.Path.Value)
		name := path[strings.LastIndex(path, "/")+1:]

		if spec.Name != nil {
			name = spec.Name.Name
		}

		imports[name] = path
	}

	return imports
}

// isNonNegativeVar reports whether ident is never negative: a variable or
// constant declared with an unsigned type, or a local variable that starts
// non-negative and is only ever increased (see isOnlyIncreased).
func isNonNegativeVar(ident *ast.Ident, fset *token.FileSet, content []byte) bool {
	if ident.Obj == nil {
		return false
	}

	switch decl := ident.Obj.Decl.(type) {
	case *ast.Field:
		return isUnsignedType(decl.Type)
	case *ast.ValueSpec:
		return isUnsignedType(decl.Type) || isNonNegativeLocalVar(decl, ident.Name, fset, content)
	case *ast.AssignStmt:
		value := definedValue(decl, ident.Name)

		return value != nil && isNonNegativeStart(value, content) && isOnlyIncreased(fset, content, decl, ident.Name)
	default:
		return false
	}
}

// isNonNegativeLocalVar reports whether the variable name that the var spec
// declares inside a function starts non-negative (its zero value, or a value
// isNonNegativeStart accepts) and is only ever increased.
func isNonNegativeLocalVar(spec *ast.ValueSpec, name string, fset *token.FileSet, content []byte) bool {
	value, ok := specValue(spec, name)

	return ok && (value == nil || isNonNegativeStart(value, content)) &&
		isLocal(fset, content, spec) && isOnlyIncreased(fset, content, spec, name)
}

// isNonNegativeStart reports whether value is a non-negative starting value
// for a counter: a basic literal (a literal is never negative; a sign is a
// unary expression), len or cap, or a call in nonNegativeFuncs or
// nonNegativeMethods.
func isNonNegativeStart(value ast.Expr, content []byte) bool {
	switch v := value.(type) {
	case *ast.BasicLit:
		return true
	case *ast.CallExpr:
		return isLenOrCapCall(v) || isNonNegativeCall(v, content)
	default:
		return false
	}
}

// specValue returns the value the var spec gives name: nil when it has no
// values (the zero value), and false when name isn't in spec or one call
// gives several names their values.
func specValue(spec *ast.ValueSpec, name string) (ast.Expr, bool) {
	for i, n := range spec.Names {
		if n.Name != name {
			continue
		}

		switch len(spec.Values) {
		case 0:
			return nil, true
		case len(spec.Names):
			return spec.Values[i], true
		}
	}

	return nil, false
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

// isLocal reports whether the var spec decl is declared inside a function, so
// every write to it is in this file: it isn't one of the file's package-level
// specs.
func isLocal(fset *token.FileSet, content []byte, decl *ast.ValueSpec) bool {
	file, reparsed := reparse(content)
	packageLevel := map[int]bool{}

	for _, d := range file.Decls {
		if gen, ok := d.(*ast.GenDecl); ok {
			for _, spec := range gen.Specs {
				packageLevel[reparsed.Position(spec.Pos()).Offset] = true
			}
		}
	}

	return !packageLevel[fset.Position(decl.Pos()).Offset]
}

// increases are the writes that can only make a variable larger: name++ and
// name += a literal.
var increases = map[token.Token]bool{token.INC: true, token.ADD_ASSIGN: true}

// isOnlyIncreased reports whether every write content makes to the variable
// name that def declares, outside def itself, increases it: name++ or
// name += a literal (never negative, see isNonNegativeStart).
func isOnlyIncreased(fset *token.FileSet, content []byte, def ast.Node, name string) bool {
	for _, write := range writesAfterDefinition(fset, content, def, name) {
		switch w := write.(type) {
		case *ast.IncDecStmt:
			if !increases[w.Tok] {
				return false
			}
		case *ast.AssignStmt:
			if _, lit := w.Rhs[0].(*ast.BasicLit); !increases[w.Tok] || !lit {
				return false
			}
		default:
			return false
		}
	}

	return true
}

// writesAfterDefinition returns the statements and expressions in content that
// write the variable name that def declares, other than def itself: assigned
// (also by a later := or a range clause), incremented, decremented or its
// address taken. The file is parsed again because a mutagen only sees the node
// it mutates.
func writesAfterDefinition(fset *token.FileSet, content []byte, def ast.Node, name string) []ast.Node {
	defOffset := fset.Position(def.Pos()).Offset
	file, reparsed := reparse(content)

	var writes []ast.Node

	// In the new tree, def is the declaration at the same offset.
	ast.Inspect(file, func(n ast.Node) bool {
		for _, target := range writeTargets(n) {
			ident, ok := target.(*ast.Ident)
			if !ok || ident.Name != name || ident.Obj == nil {
				continue
			}

			decl, ok := ident.Obj.Decl.(ast.Node)
			if ok && reparsed.Position(decl.Pos()).Offset == defOffset && n != decl {
				writes = append(writes, n)
			}
		}

		return true
	})

	return writes
}

// reparse parses content again, caching the last result: a mutagen only sees
// the node it mutates, and the checks above need the whole file. content was
// parsed once already, so this can't fail.
func reparse(content []byte) (*ast.File, *token.FileSet) {
	reparseCache.Lock()
	defer reparseCache.Unlock()

	// The cache starts with nil content; gooze only mutates files that parse,
	// so content is never empty.
	if !bytes.Equal(reparseCache.content, content) {
		fset := token.NewFileSet()
		file, _ := parser.ParseFile(fset, "", content, 0)
		reparseCache.content, reparseCache.file, reparseCache.fset = content, file, fset
	}

	return reparseCache.file, reparseCache.fset
}

// reparseCache holds the last file reparse parsed.
var reparseCache struct {
	sync.Mutex

	content []byte
	file    *ast.File
	fset    *token.FileSet
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

// countingLoopMutantOp returns token.NEQ when expr stops a counting loop, so
// its != mutant is equivalent, and token.ILLEGAL otherwise. A counter that
// starts at or below its bound and steps by one can't jump past the bound, so
// "i < bound" behaves as "i != bound" (see countsUpTo); counting down to 0,
// "i > 0" behaves as "i != 0" (see countsDownTo0).
func countingLoopMutantOp(expr *ast.BinaryExpr, fset *token.FileSet, content []byte) token.Token {
	file, reparsed := reparse(content)
	offset := fset.Position(expr.Pos()).Offset

	var loop *ast.ForStmt

	var cond *ast.BinaryExpr

	// In the new tree, expr is the conjunct of a for condition at the same
	// offset.
	ast.Inspect(file, func(n ast.Node) bool {
		if f, ok := n.(*ast.ForStmt); ok {
			for _, c := range conjuncts(f.Cond) {
				if loop == nil && reparsed.Position(c.Pos()).Offset == offset {
					loop, cond = f, c
				}
			}
		}

		return loop == nil
	})

	if loop == nil || !countsToBound(loop, cond, reparsed, content) {
		return token.ILLEGAL
	}

	return token.NEQ
}

// conjuncts returns the comparisons cond requires together: cond itself, or
// the operands of its top-level && chain.
func conjuncts(cond ast.Expr) []*ast.BinaryExpr {
	if paren, ok := cond.(*ast.ParenExpr); ok {
		return conjuncts(paren.X)
	}

	bin, ok := cond.(*ast.BinaryExpr)
	if !ok {
		return nil
	}

	// A switch, not ==, which would add the equivalent <= mutant: no boolean
	// operator sorts below &&.
	switch bin.Op {
	case token.LAND:
		return append(conjuncts(bin.X), conjuncts(bin.Y)...)
	default:
		return []*ast.BinaryExpr{bin}
	}
}

// countsToBound reports whether cond, a conjunct of loop's condition, compares
// a counter of loop with its bound: "v < bound" or "bound > v" counting up
// from 0 (see countsUpTo), or "v > 0" or "0 < v" counting down (see
// countsDownTo0).
func countsToBound(loop *ast.ForStmt, cond *ast.BinaryExpr, fset *token.FileSet, content []byte) bool {
	switch cond.Op {
	case token.LSS:
		return countsUpTo(loop, cond.X, cond.Y, fset, content) || countsDownTo0(loop, cond.Y, cond.X, fset, content)
	case token.GTR:
		return countsUpTo(loop, cond.Y, cond.X, fset, content) || countsDownTo0(loop, cond.X, cond.Y, fset, content)
	default:
		return false
	}
}

// countsUpTo reports whether counter starts at 0 and its only write is one
// step of +1 per iteration of loop (see stepsOnce), and bound never drops
// below the counter: a literal, len or cap of a variable that is never
// written, or a variable isNonNegativeVar accepts, which only grows.
func countsUpTo(loop *ast.ForStmt, counter, bound ast.Expr, fset *token.FileSet, content []byte) bool {
	v, ok := counter.(*ast.Ident)
	if !ok || v.Obj == nil {
		return false
	}

	start, ok := counterStart(v)
	if !ok || !isZero(start) || !stepsOnce(loop, v, token.INC, token.ADD_ASSIGN, fset, content) {
		return false
	}

	return isStableBound(bound, fset, content)
}

// countsDownTo0 reports whether zero is 0 and counter starts non-negative and
// its only write is one step of -1 per iteration of loop (see stepsOnce).
func countsDownTo0(loop *ast.ForStmt, counter, zero ast.Expr, fset *token.FileSet, content []byte) bool {
	v, ok := counter.(*ast.Ident)
	if !ok || v.Obj == nil || !isZero(zero) {
		return false
	}

	start, ok := counterStart(v)

	return ok && start != nil && isNonNegativeStart(start, content) && stepsOnce(loop, v, token.DEC, token.SUB_ASSIGN, fset, content)
}

// counterStart returns the value the counter v is defined with by := or a
// local var: nil for a var without a value (the zero value), and false for
// any other declaration.
func counterStart(v *ast.Ident) (ast.Expr, bool) {
	switch decl := v.Obj.Decl.(type) {
	case *ast.AssignStmt:
		value := definedValue(decl, v.Name)

		return value, value != nil
	case *ast.ValueSpec:
		return specValue(decl, v.Name)
	default:
		return nil, false
	}
}

// isZero reports whether e is the integer literal 0; a var's zero value is
// passed as nil and counts too.
func isZero(e ast.Expr) bool {
	return e == nil || isLiteral(e, "0")
}

// isLiteral reports whether e is the basic literal written as value. A switch,
// not ==, keeps the ordering mutants of string comparison away: no literal
// sorts below "0" but strings and floats such as .5, which are not 0.
func isLiteral(e ast.Expr, value string) bool {
	lit, ok := e.(*ast.BasicLit)
	if !ok {
		return false
	}

	switch lit.Value {
	case value:
		return true
	default:
		return false
	}
}

// stepsOnce reports whether the counter v has exactly one write besides its
// declaration, and it is v++ (or v--, per inc) or v += 1 (v -= 1, per add):
// either loop's post statement or a statement in loop's body that no nested
// loop or function literal repeats.
func stepsOnce(loop *ast.ForStmt, v *ast.Ident, inc, add token.Token, fset *token.FileSet, content []byte) bool {
	decl, _ := v.Obj.Decl.(ast.Node)
	writes := writesAfterDefinition(fset, content, decl, v.Name)

	if len(writes) != 1 || !isStep(writes[0], inc, add) {
		return false
	}

	return writes[0] == loop.Post || isOncePerIteration(loop.Body, writes[0])
}

// isStep reports whether write is a ++/-- of token inc, or a +=/-= of token
// add with the literal 1. Tokens are matched by switch, so no ordering mutant
// of a token comparison is generated.
func isStep(write ast.Node, inc, add token.Token) bool {
	switch w := write.(type) {
	case *ast.IncDecStmt:
		switch w.Tok {
		case inc:
			return true
		default:
			return false
		}
	case *ast.AssignStmt:
		switch w.Tok {
		case add:
			return isLiteral(w.Rhs[0], "1")
		default:
			return false
		}
	default:
		return false
	}
}

// isOncePerIteration reports whether stmt is in body outside any nested loop
// or function literal, so it runs at most once per iteration.
func isOncePerIteration(body *ast.BlockStmt, stmt ast.Node) bool {
	found := false

	ast.Inspect(body, func(n ast.Node) bool {
		switch n.(type) {
		case *ast.ForStmt, *ast.RangeStmt, *ast.FuncLit:
			return false
		}

		if n == stmt {
			found = true
		}

		return !found
	})

	return found
}

// isStableBound reports whether bound never drops below a counter that
// started at 0: a literal, len or cap of a variable that is never written, or
// a variable isNonNegativeVar accepts.
func isStableBound(bound ast.Expr, fset *token.FileSet, content []byte) bool {
	switch b := bound.(type) {
	case *ast.BasicLit:
		return true
	case *ast.CallExpr:
		if !isLenOrCapCall(b) {
			return false
		}

		// len and cap take exactly one argument.
		arg, ok := b.Args[0].(*ast.Ident)
		if !ok || arg.Obj == nil {
			return false
		}

		decl, _ := arg.Obj.Decl.(ast.Node)

		return len(writesAfterDefinition(fset, content, decl, arg.Name)) == 0
	case *ast.Ident:
		return isNonNegativeVar(b, fset, content)
	default:
		return false
	}
}
