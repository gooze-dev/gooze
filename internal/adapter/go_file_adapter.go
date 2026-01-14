package adapter

import (
	"go/ast"
	"go/parser"
	"go/token"

	m "github.com/mouse-blink/gooze/internal/model"
)

// GoFileAdapter encapsulates Go-specific parsing and scope-detection logic so
// the domain layer can focus on mutation rules while delegating compilation
// details to an infrastructure component.
type GoFileAdapter interface {
	// Parse builds an AST using the provided file set and optional source bytes.
	Parse(fileSet *token.FileSet, filename string, src []byte) (*ast.File, error)

	// ExtractScopes inspects an AST and returns the code scopes that are relevant
	// for mutation testing (global declarations, init functions, regular funcs).
	ExtractScopes(fileSet *token.FileSet, file *ast.File) []m.CodeScope

	// FunctionLines derives the line numbers of function bodies for backward
	// compatibility with older reporting formats.
	FunctionLines(scopes []m.CodeScope) []int

	// ScopeForLine returns the scope type for a given line number.
	ScopeForLine(scopes []m.CodeScope, line int) m.ScopeType
}

// LocalGoFileAdapter provides a concrete GoFileAdapter backed by go/parser.
type LocalGoFileAdapter struct{}

// NewLocalGoFileAdapter constructs a LocalGoFileAdapter.
func NewLocalGoFileAdapter() *LocalGoFileAdapter {
	return &LocalGoFileAdapter{}
}

// Parse builds an AST for the provided filename/source pair.
func (a *LocalGoFileAdapter) Parse(fileSet *token.FileSet, filename string, src []byte) (*ast.File, error) {
	return parser.ParseFile(fileSet, filename, src, parser.ParseComments)
}

// ExtractScopes inspects AST declarations and records mutation-relevant scopes.
func (a *LocalGoFileAdapter) ExtractScopes(fileSet *token.FileSet, file *ast.File) []m.CodeScope {
	var scopes []m.CodeScope

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			if d.Tok == token.CONST || d.Tok == token.VAR {
				for _, spec := range d.Specs {
					vs, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}

					start := fileSet.Position(vs.Pos()).Line
					end := fileSet.Position(vs.End()).Line

					for _, name := range vs.Names {
						scopes = append(scopes, m.CodeScope{
							Type:      m.ScopeGlobal,
							StartLine: start,
							EndLine:   end,
							Name:      name.Name,
						})
					}
				}
			}

		case *ast.FuncDecl:
			startLine := fileSet.Position(d.Pos()).Line
			endLine := fileSet.Position(d.End()).Line

			scopeType := m.ScopeFunction
			if d.Name.Name == "init" {
				scopeType = m.ScopeInit
			}

			scopes = append(scopes, m.CodeScope{
				Type:      scopeType,
				StartLine: startLine,
				EndLine:   endLine,
				Name:      d.Name.Name,
			})
		}
	}

	return scopes
}

// FunctionLines returns the starting line for each function/init scope.
func (a *LocalGoFileAdapter) FunctionLines(scopes []m.CodeScope) []int {
	var lines []int

	seen := make(map[int]struct{})

	for _, scope := range scopes {
		if scope.Type != m.ScopeFunction && scope.Type != m.ScopeInit {
			continue
		}

		if _, ok := seen[scope.StartLine]; ok {
			continue
		}

		lines = append(lines, scope.StartLine)
		seen[scope.StartLine] = struct{}{}
	}

	return lines
}

// ScopeForLine determines which scope type covers the requested line.
func (a *LocalGoFileAdapter) ScopeForLine(scopes []m.CodeScope, line int) m.ScopeType {
	for _, scope := range scopes {
		if line >= scope.StartLine && line <= scope.EndLine {
			return scope.Type
		}
	}

	return m.ScopeFunction
}
