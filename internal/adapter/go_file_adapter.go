package adapter

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"log/slog"
)

// LocalGoFileAdapter provides a concrete GoFileAdapter backed by go/parser.
type LocalGoFileAdapter struct{}

// NewLocalGoFileAdapter constructs a LocalGoFileAdapter.
func NewLocalGoFileAdapter() *LocalGoFileAdapter {
	return &LocalGoFileAdapter{}
}

// Parse builds an AST for the provided filename/source pair.
func (a *LocalGoFileAdapter) Parse(ctx context.Context, fileSet *token.FileSet, filename string, src []byte) (*ast.File, error) {
	if err := ctx.Err(); err != nil {
		slog.Warn("Terminated parse early due to context cancellation", "filename", filename, "error", err)
		return nil, err
	}

	return parser.ParseFile(fileSet, filename, src, parser.ParseComments)
}
