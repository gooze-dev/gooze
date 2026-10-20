// Package domain contains the core mutation testing workflow and logic.
package domain

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/token"

	"gooze.dev/pkg/gooze/internal/domain/mutagens"
	m "gooze.dev/pkg/gooze/internal/model"
)

// Mutagen defines the interface for mutation generation.
type Mutagen interface {
	// GenerateMutation returns every mutation for a source as a slice.
	GenerateMutation(ctx context.Context, source m.Source, mutationTypes ...m.MutationType) ([]m.Mutation, error)
	// StreamMutations invokes fn for each generated mutation without retaining
	// them, so callers can process mutations one at a time. If fn returns an
	// error, streaming stops and that error is returned.
	StreamMutations(ctx context.Context, source m.Source, fn func(m.Mutation) error, mutationTypes ...m.MutationType) error
}

// mutagen handles pure mutation generation logic.
type mutagen struct {
	GoFileAdapter
	SourceFSAdapter
	differ Differ
}

// NewMutagen creates a new Mutagen instance.
func NewMutagen(goFileAdapter GoFileAdapter, sourceFSAdapter SourceFSAdapter, differ Differ) Mutagen {
	return &mutagen{
		GoFileAdapter:   goFileAdapter,
		SourceFSAdapter: sourceFSAdapter,
		differ:          differ,
	}
}

func (mg *mutagen) GenerateMutation(ctx context.Context, source m.Source, mutationTypes ...m.MutationType) ([]m.Mutation, error) {
	mutations := make([]m.Mutation, 0)

	err := mg.StreamMutations(ctx, source, func(mutation m.Mutation) error {
		mutations = append(mutations, mutation)
		return nil
	}, mutationTypes...)
	if err != nil {
		return nil, err
	}

	return mutations, nil
}

func (mg *mutagen) StreamMutations(ctx context.Context, source m.Source, fn func(m.Mutation) error, mutationTypes ...m.MutationType) error {
	if err := validateSource(source); err != nil {
		return err
	}

	mutationTypes, err := resolveMutationTypes(mutationTypes)
	if err != nil {
		return err
	}

	if err := validateAdapters(mg); err != nil {
		return err
	}

	content, fset, file, err := mg.loadSourceAST(ctx, source)
	if err != nil {
		return err
	}

	ignore := buildIgnoreIndex(file, fset, content)

	for _, mutationType := range mutationTypes {
		if err := walkMutations(mutationType, file, fset, content, source, ignore, mg.differ, fn); err != nil {
			return err
		}
	}

	return nil
}

func validateSource(source m.Source) error {
	if source.Origin == nil || source.Origin.FullPath == "" {
		return fmt.Errorf("missing source origin")
	}

	return nil
}

func validateAdapters(mg *mutagen) error {
	if mg.SourceFSAdapter == nil || mg.GoFileAdapter == nil || mg.differ == nil {
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

// skips reports whether n is excluded from mutationType by an ignore
// directive, and if so whether its children should still be visited.
func (ix ignoreIndex) skips(n ast.Node, fset *token.FileSet, mutationType m.MutationType) (skip, descend bool) {
	if fd, ok := n.(*ast.FuncDecl); ok {
		if rule, ok := ix.funcByPos[fd.Pos()]; ok && rule.ignores(mutationType) {
			return true, false
		}
	}

	line := fset.PositionFor(n.Pos(), true).Line
	if rule, ok := ix.line[line]; ok && rule.ignores(mutationType) {
		return true, true
	}

	return false, false
}

// walkMutations visits the file once for mutationType and hands each mutation
// to fn as soon as it is generated, completing its line and diff only then.
// The walk stops at the first error fn returns.
func walkMutations(
	mutationType m.MutationType,
	file *ast.File,
	fset *token.FileSet,
	content []byte,
	source m.Source,
	ignore ignoreIndex,
	differ Differ,
	fn func(m.Mutation) error,
) error {
	if ignore.file.ignores(mutationType) {
		return nil
	}

	var walkErr error

	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil || walkErr != nil {
			return false
		}

		if skip, descend := ignore.skips(n, fset, mutationType); skip {
			return descend
		}

		for _, mutation := range generateMutationsForNode(mutationType, n, fset, content, source) {
			mutation.Line = lineForOffset(content, firstDifference(content, mutation.MutatedCode))
			mutation.DiffCode = differ.Diff(content, mutation.MutatedCode)

			if walkErr = fn(mutation); walkErr != nil {
				return false
			}
		}

		return true
	})

	return walkErr
}

// firstDifference returns the index of the first byte that differs between a and
// b, or -1 if one is a prefix of the other. This locates where a mutation begins,
// since MutatedCode shares an identical prefix with the original up to that point.
func firstDifference(a, b []byte) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}

	for i := range n {
		if a[i] != b[i] {
			return i
		}
	}

	if len(a) != len(b) {
		return n
	}

	return -1
}

// lineForOffset maps a byte offset in content to its 1-based line number,
// clamping out-of-range offsets.
func lineForOffset(content []byte, offset int) int {
	if offset < 0 {
		offset = 0
	}

	if offset > len(content) {
		offset = len(content)
	}

	return 1 + bytes.Count(content[:offset], []byte{'\n'})
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
