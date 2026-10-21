package domain

import (
	"go/ast"
	"go/token"
)

// funcSpan is the line range of one function declaration and the names a
// --run pattern can match it by.
type funcSpan struct {
	start, end int
	names      []string
}

// funcIndex locates the function declaration around a line.
type funcIndex []funcSpan

// buildFuncIndex lists the file's function declarations. A function is named
// "Name"; a method both "Name" and "Type.Name", whatever its receiver form.
func buildFuncIndex(file *ast.File, fset *token.FileSet) funcIndex {
	var index funcIndex

	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		names := []string{fd.Name.Name}
		if recv := receiverTypeName(fd); recv != "" {
			names = append(names, recv+"."+fd.Name.Name)
		}

		index = append(index, funcSpan{
			start: fset.Position(fd.Pos()).Line,
			end:   fset.Position(fd.End()).Line,
			names: names,
		})
	}

	return index
}

// receiverTypeName returns the receiver's type name without pointer or type
// parameters (Parser for *Parser, Gen for Gen[T]), or "" for a function.
func receiverTypeName(fd *ast.FuncDecl) string {
	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return ""
	}

	expr := fd.Recv.List[0].Type
	for {
		switch t := expr.(type) {
		case *ast.StarExpr:
			expr = t.X
		case *ast.IndexExpr:
			expr = t.X
		case *ast.IndexListExpr:
			expr = t.X
		case *ast.Ident:
			return t.Name
		default:
			return ""
		}
	}
}

// at returns the names of the function declared around line, or nil outside
// any function.
func (ix funcIndex) at(line int) []string {
	for _, span := range ix {
		if line >= span.start && line <= span.end {
			return span.names
		}
	}

	return nil
}
