// Package domain contains the core mutation testing workflow and logic.
package domain

import (
	"bytes"
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
	// GenerateMutation returns every mutation for a source as a slice.
	GenerateMutation(ctx context.Context, source m.Source, mutationTypes ...m.MutationType) ([]m.Mutation, error)
	// StreamMutations invokes fn for each generated mutation without retaining
	// them, so callers can process mutations one at a time. If fn returns an
	// error, streaming stops and that error is returned.
	StreamMutations(ctx context.Context, source m.Source, fn func(m.Mutation) error, mutationTypes ...m.MutationType) error
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

	for _, mutationType := range mutationTypes {
		for _, mutation := range collectMutations(mutationType, file, fset, content, source) {
			if err := fn(mutation); err != nil {
				return err
			}
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

		nodeMutations := generateMutationsForNode(mutationType, n, fset, content, source)
		for i := range nodeMutations {
			nodeMutations[i].Line = lineForOffset(content, firstDifference(content, nodeMutations[i].MutatedCode))
		}

		mutations = append(mutations, nodeMutations...)

		return true
	})

	return mutations
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
