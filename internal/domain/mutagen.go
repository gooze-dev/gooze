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
	GenerateMutation(ctx context.Context, source m.Source, mutationTypes ...m.MutationType) ([]m.Mutation, error)
	StreamMutations(ctx context.Context, source <-chan m.Source, threads int, mutationTypes ...m.MutationType) (<-chan m.Mutation, <-chan error)
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

func (mg *mutagen) GenerateMutation(ctx context.Context, source m.Source, mutationTypes ...m.MutationType) ([]m.Mutation, error) {
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

// StreamMutations streams mutations for sources received from a channel.
// It returns a channel of mutations and a channel for errors.
func (mg *mutagen) StreamMutations(ctx context.Context, sources <-chan m.Source, threads int, mutationTypes ...m.MutationType) (<-chan m.Mutation, <-chan error) {
	bufferSize := threads
	if bufferSize <= 0 {
		bufferSize = 1
	}

	mutationCh := make(chan m.Mutation, bufferSize)
	errCh := make(chan error, 1)

	go func() {
		defer close(mutationCh)
		defer close(errCh)

		// Stage 1: Validate configuration
		resolvedTypes, err := mg.validateConfig(mutationTypes)
		if err != nil {
			errCh <- err
			return
		}

		// Stage 2: Process sources and stream mutations
		mg.processSourcesStream(ctx, sources, resolvedTypes, mutationCh, errCh)
	}()

	return mutationCh, errCh
}

// validateConfig validates adapters and resolves mutation types.
func (mg *mutagen) validateConfig(mutationTypes []m.MutationType) ([]m.MutationType, error) {
	resolvedTypes, err := resolveMutationTypes(mutationTypes)
	if err != nil {
		return nil, err
	}

	if err := validateAdapters(mg); err != nil {
		return nil, err
	}

	return resolvedTypes, nil
}

// processSourcesStream processes sources from channel and sends mutations to output channel.
func (mg *mutagen) processSourcesStream(ctx context.Context, sources <-chan m.Source, mutationTypes []m.MutationType, mutationCh chan<- m.Mutation, errCh chan<- error) {
	for source := range sources {
		if ctx.Err() != nil {
			errCh <- ctx.Err()
			return
		}

		if !mg.processSourceStream(ctx, source, mutationTypes, mutationCh, errCh) {
			return
		}
	}
}

// processSourceStream processes a single source and sends its mutations to the channel.
// Returns false if processing should stop.
func (mg *mutagen) processSourceStream(ctx context.Context, source m.Source, mutationTypes []m.MutationType, mutationCh chan<- m.Mutation, errCh chan<- error) bool {
	if err := validateSource(source); err != nil {
		errCh <- err
		return false
	}

	content, fset, file, err := mg.loadSourceAST(ctx, source)
	if err != nil {
		errCh <- err
		return false
	}

	return mg.streamMutationsForSource(ctx, source, content, fset, file, mutationTypes, mutationCh, errCh)
}

// streamMutationsForSource generates and streams mutations for a parsed source.
// Returns false if context was cancelled.
func (mg *mutagen) streamMutationsForSource(ctx context.Context, source m.Source, content []byte, fset *token.FileSet, file *ast.File, mutationTypes []m.MutationType, mutationCh chan<- m.Mutation, errCh chan<- error) bool {
	for _, mutationType := range mutationTypes {
		mutations := collectMutations(mutationType, file, fset, content, source)

		if !mg.sendMutations(ctx, mutations, mutationCh, errCh) {
			return false
		}
	}

	return true
}

// sendMutations sends mutations to the channel, respecting context cancellation.
// Returns false if context was cancelled.
func (mg *mutagen) sendMutations(ctx context.Context, mutations []m.Mutation, mutationCh chan<- m.Mutation, errCh chan<- error) bool {
	for _, mutation := range mutations {
		select {
		case <-ctx.Done():
			errCh <- ctx.Err()
			return false
		case mutationCh <- mutation:
		}
	}

	return true
}
