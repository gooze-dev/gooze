// Package domain contains the core mutation testing workflow and logic.
package domain

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/scanner"
	"go/token"
	"slices"

	"gooze.dev/pkg/gooze/internal/domain/mutagens"
	m "gooze.dev/pkg/gooze/internal/model"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// Mutagen defines the interface for mutation generation.
type Mutagen interface {
	// GenerateMutation returns every mutation for a source as a slice.
	GenerateMutation(ctx context.Context, source rpt.Source, mutationTypes ...mgen.Type) ([]m.Mutation, error)
	// StreamMutations invokes fn for each generated mutation without retaining
	// them, so callers can process mutations one at a time. If fn returns an
	// error, streaming stops and that error is returned.
	StreamMutations(ctx context.Context, source rpt.Source, fn func(m.Mutation) error, mutationTypes ...mgen.Type) error
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

func (mg *mutagen) GenerateMutation(ctx context.Context, source rpt.Source, mutationTypes ...mgen.Type) ([]m.Mutation, error) {
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

func (mg *mutagen) StreamMutations(ctx context.Context, source rpt.Source, fn func(m.Mutation) error, mutationTypes ...mgen.Type) error {
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
	funcs := buildFuncIndex(file, fset)

	for _, mutationType := range mutationTypes {
		if err := walkMutations(mutationType, file, fset, content, source, ignore, funcs, mg.differ, fn); err != nil {
			return err
		}
	}

	return nil
}

func validateSource(source rpt.Source) error {
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

func resolveMutationTypes(mutationTypes []mgen.Type) ([]mgen.Type, error) {
	if len(mutationTypes) == 0 {
		return DefaultMutations, nil
	}

	for _, mutationType := range mutationTypes {
		if _, ok := mutationGenerators[mutationType]; !ok {
			return nil, fmt.Errorf("unsupported mutation type: %s", mutationType.Name)
		}
	}

	return mutationTypes, nil
}

func (mg *mutagen) loadSourceAST(ctx context.Context, source rpt.Source) ([]byte, *token.FileSet, *ast.File, error) {
	content, err := mg.ReadFile(ctx, m.Path(source.Origin.FullPath))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to read %s: %w", source.Origin.FullPath, err)
	}

	fset := token.NewFileSet()

	file, err := mg.Parse(ctx, fset, source.Origin.FullPath, content)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to parse %s: %w", source.Origin.FullPath, err)
	}

	return content, fset, file, nil
}

// visit decides, from the ignore directives, whether n is mutated for
// mutationType and whether its children are visited.
func (ix ignoreIndex) visit(n ast.Node, fset *token.FileSet, mutationType mgen.Type) (mutate, descend bool) {
	if fd, ok := n.(*ast.FuncDecl); ok {
		if rule, ok := ix.funcByPos[fd.Pos()]; ok && rule.ignores(mutationType) {
			return false, false
		}
	}

	line := fset.PositionFor(n.Pos(), true).Line
	if rule, ok := ix.line[line]; ok && rule.ignores(mutationType) {
		return false, true
	}

	return true, true
}

// keepChanged drops the mutations that a directive ignores where they change
// the file: a line directive of the changed line, counted like the
// directives (honoring //line comments), or a file or function directive that
// lists the mutant's replacement (see replacementOf). Block mutagens
// (increment, assignment) visit a block but change one of its statements, so
// the ignore of the visited node's own line isn't enough.
func (ix ignoreIndex) keepChanged(
	mutations []m.Mutation,
	fset *token.FileSet,
	file *ast.File,
	content []byte,
	mutationType mgen.Type,
) []m.Mutation {
	tokenFile := fset.File(file.Pos())
	if tokenFile == nil {
		return mutations
	}

	return slices.DeleteFunc(mutations, func(mutation m.Mutation) bool {
		// An unchanged mutation (offset -1) has no position, so no rule.
		line := fset.PositionFor(tokenFile.Pos(firstDifference(content, mutation.MutatedCode)), true).Line
		replacement := func() string { return replacementOf(content, mutation.MutatedCode) }

		return ix.line[line].ignoresMutant(mutationType, replacement) ||
			ix.funcLine[line].ignoresMutant(mutationType, replacement) ||
			ix.file.ignoresMutant(mutationType, replacement)
	})
}

// replacementOf returns the token mutated writes where it differs from
// original: the last Go token of mutated that starts at or before the first
// difference, such as <=, --, false or 0.5, or "" when they don't differ.
func replacementOf(original, mutated []byte) string {
	offset := firstDifference(original, mutated)

	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(mutated))

	var s scanner.Scanner

	// Equivalent: the first difference is never inside a comment, so scanning
	// comments (mode 1) finds the same token.
	s.Init(file, mutated, nil, 0) //gooze:ignore numbers

	found := ""

	for {
		pos, tok, lit := s.Scan()
		if tok == token.EOF || file.Offset(pos) > offset {
			return found
		}

		found = lit
		if !tok.IsLiteral() {
			found = tok.String()
		}
	}
}

// walkMutations visits the file once for mutationType and hands each mutation
// to fn as soon as it is generated, completing its line and diff only then.
// The walk stops at the first error fn returns.
func walkMutations(
	mutationType mgen.Type,
	file *ast.File,
	fset *token.FileSet,
	content []byte,
	source rpt.Source,
	ignore ignoreIndex,
	funcs funcIndex,
	differ Differ,
	fn func(m.Mutation) error,
) error {
	if ignore.file.ignores(mutationType) {
		return nil
	}

	var walkErr error

	// Nodes the walk skips: type constraints, and subtrees whose every mutant
	// is equivalent.
	constraints := map[ast.Node]bool{}
	markEquivalentSubtrees(file, constraints)

	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil || walkErr != nil {
			return false
		}

		if markTypeParams(n, constraints); constraints[n] {
			return false
		}

		mutate, descend := ignore.visit(n, fset, mutationType)
		if !mutate {
			return descend
		}

		mutations := ignore.keepChanged(generateMutationsForNode(mutationType, n, fset, content, source), fset, file, content, mutationType)
		for _, mutation := range mutations {
			mutation.Line = lineForOffset(content, firstDifference(content, mutation.MutatedCode))
			mutation.Funcs = funcs.at(mutation.Line)
			mutation.DiffCode = differ.Diff(content, mutation.MutatedCode)

			if walkErr = fn(mutation); walkErr != nil {
				break
			}
		}

		return descend && walkErr == nil
	})

	return walkErr
}

// markTypeParams adds the type parameter list of a generic function or type
// to constraints, so the walk skips it: a constraint such as int | string is
// a type, and no mutation of it compiles. Interfaces are skipped for the same
// reason.
func markTypeParams(n ast.Node, constraints map[ast.Node]bool) {
	switch node := n.(type) {
	case *ast.FuncType:
		if node.TypeParams != nil {
			constraints[node.TypeParams] = true
		}
	case *ast.TypeSpec:
		if node.TypeParams != nil {
			constraints[node.TypeParams] = true
		}
	case *ast.InterfaceType:
		constraints[node] = true
	}
}

// firstDifference returns the index of the first byte that differs between a and
// b, or -1 if one is a prefix of the other. This locates where a mutation begins,
// since MutatedCode shares an identical prefix with the original up to that point.
func firstDifference(a, b []byte) int {
	n := min(len(b), len(a))

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

var mutationGenerators = map[mgen.Type]func(ast.Node, *token.FileSet, []byte, rpt.Source) []m.Mutation{
	mgen.Arithmetic: mutagens.GenerateArithmeticMutations,
	mgen.Boolean:    mutagens.GenerateBooleanMutations,
	mgen.Numbers:    mutagens.GenerateNumberMutations,
	mgen.Comparison: mutagens.GenerateComparisonMutations,
	mgen.Logical:    mutagens.GenerateLogicalMutations,
	mgen.Unary:      mutagens.GenerateUnaryMutations,
	mgen.Bitwise:    mutagens.GenerateBitwiseMutations,
	mgen.Increment:  mutagens.GenerateIncrementMutations,
	mgen.Assignment: mutagens.GenerateAssignmentMutations,
	mgen.Branch:     mutagens.GenerateBranchMutations,
	mgen.Statement:  mutagens.GenerateStatementMutations,
	mgen.Loop:       mutagens.GenerateLoopMutations,
}

func generateMutationsForNode(
	mutationType mgen.Type,
	n ast.Node,
	fset *token.FileSet,
	content []byte,
	source rpt.Source,
) []m.Mutation {
	gen, ok := mutationGenerators[mutationType]
	if !ok {
		return nil
	}

	return gen(n, fset, content, source)
}
