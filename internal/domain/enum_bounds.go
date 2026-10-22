package domain

import (
	"context"
	"go/ast"
	"go/constant"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	m "gooze.dev/pkg/gooze/internal/model"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// isEnumBoundMutant reports whether the mutant of mutationType whose mutated
// code differs from the original at offset is one of ix.enumBounds. Their
// offsets are of the operator they replace, so the operator the mutant
// writes is the token at offset.
func (ix ignoreIndex) isEnumBoundMutant(mutationType mgen.Type, offset int, mutated []byte) bool {
	op, ok := ix.enumBounds[offset]

	return ok && mutationType == mgen.Comparison && op == tokenAt(mutated[offset:], 0)
}

// enumBoundMutants returns, by the offset of their operator, the comparison
// mutants of file that are equivalent when each enum's lowest and highest
// values bound its type: v == Lowest written as v <= Lowest, v != Highest as
// v < Highest, and so on (see enums.boundMutant). It returns nil when a file
// of the package can't be read or parsed.
func (mg *mutagen) enumBoundMutants(ctx context.Context, source rpt.Source, fset *token.FileSet, file *ast.File) map[int]string {
	files, ok := mg.packageFiles(ctx, source, file)
	if !ok {
		return nil
	}

	enums := packageEnums(files)
	specs := topLevelValueSpecs(file)
	tokenFile := fset.File(file.Pos())
	mutants := map[int]string{}

	ast.Inspect(file, func(n ast.Node) bool {
		// An expression with no bound mutant maps to "", which no mutant
		// writes.
		if expr, ok := n.(*ast.BinaryExpr); ok {
			mutants[tokenFile.Offset(expr.OpPos)] = enums.boundMutant(expr, specs)
		}

		return true
	})

	return mutants
}

// packageFiles returns file and the other files of its package: the .go files
// next to source with the same package clause, so tests of the package
// count and external tests (package p_test) don't.
func (mg *mutagen) packageFiles(ctx context.Context, source rpt.Source, file *ast.File) ([]*ast.File, bool) {
	files := []*ast.File{file}
	self := filepath.Clean(source.Origin.FullPath)

	err := mg.Walk(ctx, m.Path(filepath.Dir(self)), false, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || filepath.Clean(path) == self {
			return err
		}

		content, err := mg.ReadFile(ctx, m.Path(path))
		if err != nil {
			return err
		}

		other, err := mg.Parse(ctx, token.NewFileSet(), path, content)
		if err != nil {
			return err
		}

		if other.Name.Name == file.Name.Name {
			files = append(files, other)
		}

		return nil
	})

	return files, err == nil
}

// enumConst is a named constant of an enum: its type and value.
type enumConst struct {
	typ   string
	value int64
}

// enums are the enums of a package: its defined integer types with at least
// two named constants, each a literal or iota expression. The zero value of
// a type counts as one of its values, since a variable of the type starts
// there. A type the package converts a value to, as T(n), is left out, as is
// one with a constant this can't evaluate.
type enums struct {
	// consts are the named constants of the enums.
	consts map[string]enumConst
	// bounds are the lowest and highest value of each enum.
	bounds map[string][2]int64
	// names are every package-level constant.
	names map[string]bool
}

// packageEnums finds the enums declared in files, the files of one package.
func packageEnums(files []*ast.File) enums {
	types := integerTypes(files)
	typed := map[string]enumConst{}
	values := map[string][]int64{}
	names := map[string]bool{}

	forEachConst(files, func(name *ast.Ident, typ, value ast.Expr, index int64) {
		names[name.Name] = true

		typeName, ok := enumType(typ, types)
		if !ok {
			return
		}

		v, ok := evalConst(value, index)
		if !ok {
			types[typeName] = false
			return
		}

		values[typeName] = append(values[typeName], v)
		if name.Name != "_" {
			typed[name.Name] = enumConst{typ: typeName, value: v}
		}
	})

	// An untyped constant computed from a typed one, such as A + 1, has the
	// type too, with a value this doesn't track.
	forEachConst(files, func(_ *ast.Ident, typ, value ast.Expr, _ int64) {
		if _, ok := enumType(typ, types); !ok {
			ast.Inspect(value, func(n ast.Node) bool {
				if ident, ok := n.(*ast.Ident); ok {
					if c, ok := typed[ident.Name]; ok {
						types[c.typ] = false
					}
				}

				return true
			})
		}
	})

	markOutOfRange(files, types, typed)

	return newEnums(typed, values, types, names)
}

// newEnums keeps the constants of the types still enums with at least two
// named constants, and the bounds of those types.
func newEnums(typed map[string]enumConst, values map[string][]int64, types map[string]bool, names map[string]bool) enums {
	e := enums{consts: map[string]enumConst{}, bounds: map[string][2]int64{}, names: names}
	count := map[string]int{}

	for _, c := range typed {
		count[c.typ]++
	}

	for typ, n := range count {
		if n < 2 || !types[typ] {
			continue
		}

		bounds := [2]int64{0, 0}
		for _, v := range values[typ] {
			bounds = [2]int64{min(bounds[0], v), max(bounds[1], v)}
		}

		e.bounds[typ] = bounds
	}

	for name, c := range typed {
		if _, ok := e.bounds[c.typ]; ok {
			e.consts[name] = c
		}
	}

	return e
}

// integerTypes returns the package-level types of files defined as a
// predeclared integer type, such as type Kind int, each mapped to true.
func integerTypes(files []*ast.File) map[string]bool {
	types := map[string]bool{}

	for _, file := range files {
		for _, decl := range file.Decls {
			if gen, ok := decl.(*ast.GenDecl); ok {
				for _, spec := range gen.Specs {
					if ts, ok := spec.(*ast.TypeSpec); ok && isIntegerType(ts.Type) && ts.TypeParams == nil && !ts.Assign.IsValid() {
						types[ts.Name.Name] = true
					}
				}
			}
		}
	}

	return types
}

// isIntegerType reports whether e is a predeclared integer type.
func isIntegerType(e ast.Expr) bool {
	ident, ok := e.(*ast.Ident)
	if !ok || ident.Obj != nil {
		return false
	}

	switch ident.Name {
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr", "byte", "rune":
		return true
	default:
		return false
	}
}

// forEachConst calls fn for each package-level constant of files with its
// type and value expressions, repeating the last ones for a spec that omits
// them as Go does, and the index of its spec, the value of iota there.
func forEachConst(files []*ast.File, fn func(name *ast.Ident, typ, value ast.Expr, index int64)) {
	for _, file := range files {
		for _, decl := range file.Decls {
			// A switch, not gen.Tok == token.CONST, which would add equivalent
			// <= and >= mutants: import specs aren't value specs.
			if gen, ok := decl.(*ast.GenDecl); ok {
				switch gen.Tok { //nolint:gocritic,exhaustive // see above
				case token.CONST:
					forEachSpecConst(gen, fn)
				}
			}
		}
	}
}

// forEachSpecConst is forEachConst for the constants of one declaration.
func forEachSpecConst(gen *ast.GenDecl, fn func(name *ast.Ident, typ, value ast.Expr, index int64)) {
	var (
		typ    ast.Expr
		values []ast.Expr
	)

	for index, spec := range gen.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}

		if vs.Values != nil {
			typ, values = vs.Type, vs.Values
		}

		for i, name := range vs.Names {
			if i < len(values) {
				fn(name, typ, values[i], int64(index))
			}
		}
	}
}

// enumType returns the name of the type typ when it is one of types.
func enumType(typ ast.Expr, types map[string]bool) (string, bool) {
	ident, ok := typ.(*ast.Ident)
	if !ok {
		return "", false
	}

	_, ok = types[ident.Name]

	return ident.Name, ok
}

// markOutOfRange sets to false each of types whose values files may take
// outside its constants: a type they convert a value to, as T(n), and one
// whose constant they compute with, as Last + 1 or -Last.
func markOutOfRange(files []*ast.File, types map[string]bool, typed map[string]enumConst) {
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.CallExpr:
				if ident, ok := ast.Unparen(node.Fun).(*ast.Ident); ok {
					if _, ok := types[ident.Name]; ok {
						types[ident.Name] = false
					}
				}
			case *ast.BinaryExpr:
				if !isComparison(node.Op) {
					markConstOperand(node.X, types, typed)
					markConstOperand(node.Y, types, typed)
				}
			case *ast.UnaryExpr:
				markConstOperand(node.X, types, typed)
			}

			return true
		})
	}
}

// markConstOperand sets the type of the enum constant operand names to false.
func markConstOperand(operand ast.Expr, types map[string]bool, typed map[string]enumConst) {
	if ident, ok := ast.Unparen(operand).(*ast.Ident); ok {
		if c, ok := typed[ident.Name]; ok {
			types[c.typ] = false
		}
	}
}

// isComparison reports whether op is a comparison operator.
func isComparison(op token.Token) bool {
	switch op {
	case token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ:
		return true
	default:
		return false
	}
}

// evalConst returns the value of a constant expression made of integer
// or rune literals, iota (index), parentheses, signs and +, -, * and <<.
func evalConst(e ast.Expr, index int64) (int64, bool) {
	switch v := e.(type) {
	case *ast.BasicLit:
		return constant.Int64Val(constant.ToInt(constant.MakeFromLiteral(v.Value, v.Kind, 0)))
	case *ast.Ident:
		return index, isPredeclared(v, "iota")
	case *ast.ParenExpr:
		return evalConst(v.X, index)
	case *ast.UnaryExpr:
		x, ok := evalConst(v.X, index)
		return evalUnary(v.Op, x, ok)
	case *ast.BinaryExpr:
		x, okX := evalConst(v.X, index)
		y, okY := evalConst(v.Y, index)

		return evalBinary(v.Op, x, y, okX && okY)
	default:
		return 0, false
	}
}

func evalUnary(op token.Token, x int64, ok bool) (int64, bool) {
	switch op {
	case token.SUB:
		return -x, ok
	case token.ADD:
		return x, ok
	default:
		return 0, false
	}
}

func evalBinary(op token.Token, x, y int64, ok bool) (int64, bool) {
	switch op {
	case token.ADD:
		return x + y, ok
	case token.SUB:
		return x - y, ok
	case token.MUL:
		return x * y, ok
	case token.SHL:
		if y < 0 || y > 62 {
			return 0, false
		}

		return x << y, ok
	default:
		return 0, false
	}
}

// topLevelValueSpecs returns the package-level var and const specs of file.
func topLevelValueSpecs(file *ast.File) map[any]bool {
	specs := map[any]bool{}

	for _, decl := range file.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok {
			for _, spec := range gen.Specs {
				specs[spec] = true
			}
		}
	}

	return specs
}

// boundMutant returns the comparison operator whose mutant of expr is
// equivalent when the enum constant it compares with is its type's bound, or
// "": for v == Lowest, <= (v can't be below Lowest); for v != Lowest, >; for
// v == Highest, >=; for v != Highest, <; and the mirrored operators with the
// constant on the left. The other operand must be an identifier, field or
// call, not a constant. specs are the package-level specs of the file, to
// tell the package's constants from local names that shadow them.
func (e enums) boundMutant(expr *ast.BinaryExpr, specs map[any]bool) string {
	if c, ok := e.constant(expr.Y, specs); ok && e.isVariable(expr.X) {
		return e.boundOp(expr.Op, c,
			map[token.Token]string{token.EQL: "<=", token.NEQ: ">"},
			map[token.Token]string{token.EQL: ">=", token.NEQ: "<"})
	}

	if c, ok := e.constant(expr.X, specs); ok && e.isVariable(expr.Y) {
		return e.boundOp(expr.Op, c,
			map[token.Token]string{token.EQL: ">=", token.NEQ: "<"},
			map[token.Token]string{token.EQL: "<=", token.NEQ: ">"})
	}

	return ""
}

// boundOp returns the operator lowest maps op to when c is its type's lowest
// value, the one highest maps it to when c is the highest, or "".
func (e enums) boundOp(op token.Token, c enumConst, lowest, highest map[token.Token]string) string {
	switch c.value {
	case e.bounds[c.typ][0]:
		return lowest[op]
	case e.bounds[c.typ][1]:
		return highest[op]
	default:
		return ""
	}
}

// constant returns the enum constant x names, unless x is a local name.
func (e enums) constant(x ast.Expr, specs map[any]bool) (enumConst, bool) {
	ident, ok := x.(*ast.Ident)
	if !ok || ident.Obj != nil && !specs[ident.Obj.Decl] {
		return enumConst{}, false
	}

	c, ok := e.consts[ident.Name]

	return c, ok
}

// isVariable reports whether x is a variable, a field or a call: an
// identifier of a variable declared in the file or not declared in it and
// not a constant of the package.
func (e enums) isVariable(x ast.Expr) bool {
	switch v := x.(type) {
	case *ast.Ident:
		return !e.names[v.Name] && (v.Obj == nil || v.Obj.Kind == ast.Var)
	case *ast.SelectorExpr, *ast.CallExpr:
		return true
	default:
		return false
	}
}
