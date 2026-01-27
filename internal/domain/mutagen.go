// Package domain contains the core mutation testing workflow and logic.
package domain

import (
	"fmt"
	"go/ast"
	"go/token"

	"github.com/mouse-blink/gooze/internal/adapter"
	"github.com/mouse-blink/gooze/internal/domain/mutagens"
	m "github.com/mouse-blink/gooze/internal/model"
)

// Mutagen defines the interface for mutation generation.
type Mutagen interface {
	GenerateMutation(source m.Source, mutationTypes ...m.MutationType) ([]m.Mutation, error)
}

// mutagen handles pure mutation generation logic.
type mutagen struct {
	adapter.GoFileAdapter
	adapter.SourceFSAdapter
}

// NewMutagen creates a new Mutagen instance.
func NewMutagen(goFileAdapter adapter.GoFileAdapter, sourceFSAdapter adapter.SourceFSAdapter) Mutagen {
	return &mutagen{
		GoFileAdapter:   goFileAdapter,
		SourceFSAdapter: sourceFSAdapter,
	}
}

func (mg *mutagen) GenerateMutation(source m.Source, mutationTypes ...m.MutationType) ([]m.Mutation, error) {
	if err := validateSource(source); err != nil {
		return nil, err
	}

	mutationTypes, err := resolveMutationTypes(mutationTypes)
	if err != nil {
		return nil, err
	}

	if err := validateAdapters(mg); err != nil {
		return nil, err
	}

	content, fset, file, err := mg.loadSourceAST(source)
	if err != nil {
		return nil, err
	}

	mutations := make([]m.Mutation, 0)

	for _, mutationType := range mutationTypes {
		mutations = append(mutations, collectMutations(mutationType, file, fset, content, source)...)
	}

	return mutations, nil
}

func validateSource(source m.Source) error {
	if source.Origin == nil || source.Origin.FullPath == "" {
		return fmt.Errorf("missing source origin")
	}

	return nil
}

func validateAdapters(mg *mutagen) error {
	if mg.SourceFSAdapter == nil || mg.GoFileAdapter == nil {
		return fmt.Errorf("missing adapters")
	}

	return nil
}

func resolveMutationTypes(mutationTypes []m.MutationType) ([]m.MutationType, error) {
	if len(mutationTypes) == 0 {
		return []m.MutationType{m.MutationArithmetic, m.MutationBoolean, m.MutationNumbers, m.MutationComparison, m.MutationLogical, m.MutationUnary, m.MutationBranch}, nil
	}

	for _, mutationType := range mutationTypes {
		if mutationType != m.MutationArithmetic && mutationType != m.MutationBoolean && mutationType != m.MutationNumbers && mutationType != m.MutationComparison && mutationType != m.MutationLogical && mutationType != m.MutationUnary && mutationType != m.MutationBranch {
			return nil, fmt.Errorf("unsupported mutation type: %s", mutationType.Name)
		}
	}

	return mutationTypes, nil
}

func (mg *mutagen) loadSourceAST(source m.Source) ([]byte, *token.FileSet, *ast.File, error) {
	content, err := mg.ReadFile(source.Origin.FullPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to read %s: %w", source.Origin.FullPath, err)
	}

	fset := token.NewFileSet()

	file, err := mg.Parse(fset, string(source.Origin.FullPath), content)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to parse %s: %w", source.Origin.FullPath, err)
	}

	return content, fset, file, nil
}

func collectMutations(mutationType m.MutationType, file *ast.File, fset *token.FileSet, content []byte, source m.Source) []m.Mutation {
	mutations := make([]m.Mutation, 0)

	ast.Inspect(file, func(n ast.Node) bool {
		switch mutationType {
		case m.MutationArithmetic:
			mutations = append(mutations, mutagens.GenerateArithmeticMutations(n, fset, content, source)...)
		case m.MutationBoolean:
			mutations = append(mutations, mutagens.GenerateBooleanMutations(n, fset, content, source)...)
		case m.MutationNumbers:
			mutations = append(mutations, mutagens.GenerateNumberMutations(n, fset, content, source)...)
		case m.MutationComparison:
			mutations = append(mutations, mutagens.GenerateComparisonMutations(n, fset, content, source)...)
		case m.MutationLogical:
			mutations = append(mutations, mutagens.GenerateLogicalMutations(n, fset, content, source)...)
		case m.MutationUnary:
			mutations = append(mutations, mutagens.GenerateUnaryMutations(n, fset, content, source)...)
		case m.MutationBranch:
			mutations = append(mutations, mutagens.GenerateBranchMutations(n, fset, content, source)...)
		case m.MutationStatement:
			mutations = append(mutations, mutagens.GenerateStatementMutations(n, fset, content, source)...)
		}

		return true
	})

	return mutations
}
