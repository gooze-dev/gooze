package adapter

import (
	"go/ast"
	"go/parser"
	"go/token"
)

// GoFileAdapter encapsulates Go-specific parsing and scope-detection logic so
// the domain layer can focus on mutation rules while delegating compilation
// details to an infrastructure component.
type GoFileAdapter interface {
	// Parse builds an AST using the provided file set and optional source bytes.
	Parse(fileSet *token.FileSet, filename string, src []byte) (*ast.File, error)
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
