// Package domain contains the core mutation testing workflow and logic.
package domain

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"

	"gooze.dev/pkg/gooze/internal/adapter"
	"gooze.dev/pkg/gooze/internal/domain/mutagens"
	m "gooze.dev/pkg/gooze/internal/model"
)

// Mutagen defines the interface for mutation generation.
type Mutagen interface {
	GenerateMutationChannel(ctx context.Context, sourceChan  <-chan m.Source, parallel int,mutationTypes ...m.MutationType) (<-chan m.Mutation, error)
	GenerateMutation(ctx context.Context, source m.Source, mutationTypes ...m.MutationType) ([]m.Mutation, error)
}

// mutagen handles pure mutation generation logic.
type mutagen struct {
	adapter.GoFileAdapter
	adapter.FilesAdapter
	// adapter.SourceFSAdapter
	m.Events
}

// NewMutagen creates a new Mutagen instance.
func NewMutagen(goFileAdapter adapter.GoFileAdapter, filesAdapter adapter.FilesAdapter, events m.Events) Mutagen {
	return &mutagen{
		GoFileAdapter:   goFileAdapter,
		FilesAdapter:    filesAdapter,
		Events:          events,
	}
}

func (mg *mutagen) GenerateMutationChannel(ctx context.Context, sourceChan <-chan m.Source, parallel int, mutationTypes ...m.MutationType) (<-chan m.Mutation, error) {
	mutationChan := make(chan m.Mutation, parallel)
	mg.Events.StartGeneratingMutations()
	go func() {
		defer close(mutationChan)

		for source := range sourceChan {
			mg.Events.GeneratingMutationsFor(source)
			content, fset, file, err := mg.loadSourceAST(ctx, source)
			if err != nil {
				mg.Events.Error(fmt.Errorf("failed to load AST for %s: %w", source.Origin.FullPath, err))
				return 
			}
			for _, mutationType := range mutationTypes {
				mg.Events.GeneratingMutationsFor(source)
				mutations := collectMutations(mutationType, file, fset, content, source)
				for _, mutation := range mutations {
					select {
					case mutationChan <- mutation:
					case <-ctx.Done():
						return
					}
				}
			}

		}
		mg.Events.FinishGeneratingMutations()
	}()

	return mutationChan, nil
}

func (mg *mutagen) GenerateMutation(ctx context.Context, source m.Source, mutationTypes ...m.MutationType) ([]m.Mutation, error) {

	mutationTypes, err := resolveMutationTypes(mutationTypes)
	if err != nil {
		return nil, err
	}

	content, fset, file, err := mg.loadSourceAST(ctx, source)
	if err != nil {
		return nil, err
	}

	mutations := make([]m.Mutation, 0)

	for _, mutationType := range mutationTypes {
		mutations = append(mutations, collectMutations(mutationType, file, fset, content, source)...)
	}

	return mutations, nil
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

func (mg *mutagen) loadSourceAST(ctx context.Context, source m.Source) ([]byte, *token.FileSet, *ast.File, error) {
	content, err := mg.ReadFile(ctx, source.Origin.FullPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to read %s: %w", source.Origin.FullPath, err)
	}

	fset := token.NewFileSet()

	file, err := mg.Parse(ctx, fset, string(source.Origin.FullPath), content)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to parse %s: %w", source.Origin.FullPath, err)
	}

	return content, fset, file, nil
}

func collectMutations(mutationType m.MutationType, file *ast.File, fset *token.FileSet, content []byte, source m.Source) []m.Mutation {
	ignore := buildIgnoreIndex(file, fset, content)
	if ignore.file.ignores(mutationType) {
		return nil
	}

	mutations := make([]m.Mutation, 0)

	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil {
			return true
		}

		// Function-level ignore: if the annotation is directly above the func decl,
		// skip traversing the function body entirely.
		if fd, ok := n.(*ast.FuncDecl); ok {
			if rule, ok := ignore.funcByPos[fd.Pos()]; ok && rule.ignores(mutationType) {
				return false
			}
		}

		// Line-level ignore: if the annotation is on the same line (trailing) or
		// on the line above (leading), skip generating mutations for this node.
		line := fset.PositionFor(n.Pos(), true).Line
		if rule, ok := ignore.line[line]; ok && rule.ignores(mutationType) {
			return true
		}

		mutations = append(mutations, generateMutationsForNode(mutationType, n, fset, content, source)...)

		return true
	})

	return mutations
}

var mutationGenerators = map[m.MutationType]func(ast.Node, *token.FileSet, []byte, m.Source) []m.Mutation{
	m.MutationArithmetic: mutagens.GenerateArithmeticMutations,
	m.MutationBoolean:    mutagens.GenerateBooleanMutations,
	m.MutationNumbers:    mutagens.GenerateNumberMutations,
	m.MutationComparison: mutagens.GenerateComparisonMutations,
	m.MutationLogical:    mutagens.GenerateLogicalMutations,
	m.MutationUnary:      mutagens.GenerateUnaryMutations,
	m.MutationBranch:     mutagens.GenerateBranchMutations,
	m.MutationStatement:  mutagens.GenerateStatementMutations,
	m.MutationLoop:       mutagens.GenerateLoopMutations,
}

func generateMutationsForNode(
	mutationType m.MutationType,
	n ast.Node,
	fset *token.FileSet,
	content []byte,
	source m.Source,
) []m.Mutation {
	gen, ok := mutationGenerators[mutationType]
	if !ok {
		return nil
	}

	return gen(n, fset, content, source)
}
