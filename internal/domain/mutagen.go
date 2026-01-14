// Package domain contains the core mutation testing workflow and logic.
package domain

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"

	"github.com/mouse-blink/gooze/internal/domain/mutagens"
	m "github.com/mouse-blink/gooze/internal/model"
)

// Mutagen defines the interface for mutation generation.
type Mutagen interface {
	GenerateMutations(source m.Source, mutationTypes ...m.MutationType) ([]m.Mutation, error)
	EstimateMutations(source m.Source, mutationTypes ...m.MutationType) (int, error)
}

// mutagen handles pure mutation generation logic.
type mutagen struct{}

// NewMutagen creates a new Mutagen instance.
func NewMutagen() Mutagen {
	return &mutagen{}
}

// GenerateMutations analyzes a source file and generates mutations based on type.
// If no types are specified, generates mutations for all supported types.
func (mg *mutagen) GenerateMutations(sources m.Source, mutationTypes ...m.MutationType) ([]m.Mutation, error) {
	// Default to all types if none specified
	if len(mutationTypes) == 0 {
		mutationTypes = []m.MutationType{m.MutationArithmetic, m.MutationBoolean}
	}

	// Validate mutation types
	for _, mutationType := range mutationTypes {
		if mutationType != m.MutationArithmetic && mutationType != m.MutationBoolean {
			return nil, fmt.Errorf("unsupported mutation type: %v", mutationType)
		}
	}

	source := sources
	// Parse the source file
	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, string(source.Origin), nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", source.Origin, err)
	}

	var mutations []m.Mutation

	mutationID := 0

	// Walk the AST and find mutations for each requested type
	for _, mutationType := range mutationTypes {
		ast.Inspect(file, func(n ast.Node) bool {
			switch mutationType {
			case m.MutationArithmetic:
				mutations = append(mutations, mutagens.ProcessArithmeticMutations(n, fset, source, &mutationID)...)
			case m.MutationBoolean:
				mutations = append(mutations, mutagens.ProcessBooleanMutations(n, fset, source, &mutationID)...)
			}

			return true
		})
	}

	return mutations, nil
}

// EstimateMutations calculates the total number of mutations for a source and mutation type.
func (mg *mutagen) EstimateMutations(source m.Source, mutationTypes ...m.MutationType) (int, error) {
	// Default to all types if none specified
	if len(mutationTypes) == 0 {
		mutationTypes = []m.MutationType{m.MutationArithmetic, m.MutationBoolean}
	}

	// Validate mutation types
	for _, mt := range mutationTypes {
		if mt != m.MutationArithmetic && mt != m.MutationBoolean {
			return 0, fmt.Errorf("unsupported mutation type: %v", mt)
		}
	}

	mutations, err := mg.GenerateMutations(source, mutationTypes...)
	if err != nil {
		return 0, fmt.Errorf("failed to estimate mutations for %s: %w", source.Origin, err)
	}

	return len(mutations), nil
}
