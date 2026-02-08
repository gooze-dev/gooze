package domain

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gooze.dev/pkg/gooze/internal/adapter"
	m "gooze.dev/pkg/gooze/internal/model"
)

func TestMutagen_GenerateMutation_ArithmeticBasic(t *testing.T) {
	mg := newTestMutagen()

	source := makeSourceV2(t, filepath.Join("..", "..", "examples", "basic", "main.go"))
	original := readFileBytes(t, source.Origin.FullPath)

	mutations, err := mg.GenerateMutation(context.Background(), source, m.MutationArithmetic)
	if err != nil {
		t.Fatalf("GenerateMutation failed: %v", err)
	}

	if len(mutations) != 4 {
		t.Fatalf("expected 4 mutations for +, got %d", len(mutations))
	}

	expectedOps := map[string]bool{"-": false, "*": false, "/": false, "%": false}

	for i, mutation := range mutations {
		if len(mutation.ID) == 0 {
			t.Fatalf("expected non-empty mutation ID for mutation %d", i)
		}
		if mutation.Type != m.MutationArithmetic {
			t.Fatalf("expected arithmetic mutation, got %v", mutation.Type)
		}
		if bytes.Equal(mutation.MutatedCode, original) {
			t.Fatalf("expected mutated code to differ from original")
		}

		code := string(mutation.MutatedCode)
		for op := range expectedOps {
			if strings.Contains(code, "3"+op+"5") {
				expectedOps[op] = true
			}
		}
	}

	for op, found := range expectedOps {
		if !found {
			t.Errorf("expected mutation to %s, but not found", op)
		}
	}
}

func TestMutagen_GenerateMutation_BooleanLiterals(t *testing.T) {
	mg := newTestMutagen()

	source := makeSourceV2(t, filepath.Join("..", "..", "examples", "boolean", "main.go"))
	original := readFileBytes(t, source.Origin.FullPath)

	mutations, err := mg.GenerateMutation(context.Background(), source, m.MutationBoolean)
	if err != nil {
		t.Fatalf("GenerateMutation failed: %v", err)
	}

	if len(mutations) != 4 {
		t.Fatalf("expected 4 boolean mutations, got %d", len(mutations))
	}

	expectedFragments := map[string]bool{
		"isValid := false":          false,
		"isComplete := true":        false,
		"checkStatus(false, false)": false,
		"checkStatus(true, true)":   false,
	}

	for i, mutation := range mutations {
		if len(mutation.ID) == 0 {
			t.Fatalf("expected non-empty mutation ID for mutation %d", i)
		}
		if mutation.Type != m.MutationBoolean {
			t.Fatalf("expected boolean mutation, got %v", mutation.Type)
		}
		if bytes.Equal(mutation.MutatedCode, original) {
			t.Fatalf("expected mutated code to differ from original")
		}

		code := string(mutation.MutatedCode)
		for fragment := range expectedFragments {
			if strings.Contains(code, fragment) {
				expectedFragments[fragment] = true
			}
		}
	}

	for fragment, found := range expectedFragments {
		if !found {
			t.Errorf("expected mutated code to contain %q", fragment)
		}
	}
}

func TestMutagen_GenerateMutation_DefaultTypes(t *testing.T) {
	mg := newTestMutagen()

	source := makeSourceV2(t, filepath.Join("..", "..", "examples", "basic", "main.go"))
	mutations, err := mg.GenerateMutation(context.Background(), source)
	if err != nil {
		t.Fatalf("GenerateMutation failed: %v", err)
	}

	if len(mutations) != 12 {
		t.Fatalf("expected 12 mutations, got %d", len(mutations))
	}
}

func TestMutagen_GenerateMutation_InvalidType(t *testing.T) {
	mg := newTestMutagen()

	source := makeSourceV2(t, filepath.Join("..", "..", "examples", "basic", "main.go"))
	_, err := mg.GenerateMutation(context.Background(), source, m.MutationType{Name: "invalid", Version: 1})
	if err == nil {
		t.Fatalf("expected error for invalid mutation type")
	}
}

func TestMutagen_GenerateMutation_InvalidSource(t *testing.T) {
	mg := newTestMutagen()

	_, err := mg.GenerateMutation(context.Background(), m.Source{}, m.MutationArithmetic)
	if err == nil {
		t.Fatalf("expected error for missing source origin")
	}
}

func TestMutagen_GenerateMutation_Ignore_FileLevel_ByMutagenName(t *testing.T) {
	mg := newTestMutagen()

	source := makeSourceV2(t, filepath.Join("..", "..", "examples", "ignore", "file_ignore.go"))

	mutationsArithmetic, err := mg.GenerateMutation(context.Background(), source, m.MutationArithmetic)
	if err != nil {
		t.Fatalf("GenerateMutation(arithmetic) failed: %v", err)
	}
	if len(mutationsArithmetic) != 0 {
		t.Fatalf("expected 0 arithmetic mutations due to file-level ignore, got %d", len(mutationsArithmetic))
	}

	mutationsNumbers, err := mg.GenerateMutation(context.Background(), source, m.MutationNumbers)
	if err != nil {
		t.Fatalf("GenerateMutation(numbers) failed: %v", err)
	}
	if len(mutationsNumbers) == 0 {
		t.Fatalf("expected numbers mutations to still be generated")
	}
}

func TestMutagen_GenerateMutation_Ignore_FunctionLevel_AllMutagens(t *testing.T) {
	mg := newTestMutagen()

	source := makeSourceV2(t, filepath.Join("..", "..", "examples", "ignore", "func_ignore.go"))

	mutations, err := mg.GenerateMutation(context.Background(), source, m.MutationArithmetic)
	if err != nil {
		t.Fatalf("GenerateMutation failed: %v", err)
	}

	// Each arithmetic binary expr yields 4 mutations (all other arithmetic ops).
	// File has two identical exprs, but one function is ignored => only 4 mutations.
	if len(mutations) != 4 {
		t.Fatalf("expected 4 arithmetic mutations (one function ignored), got %d", len(mutations))
	}
}

func TestMutagen_GenerateMutation_Ignore_LineLevel_TrailingComment_ByMutagenName(t *testing.T) {
	mg := newTestMutagen()

	source := makeSourceV2(t, filepath.Join("..", "..", "examples", "ignore", "line_ignore.go"))

	mutations, err := mg.GenerateMutation(context.Background(), source, m.MutationArithmetic)
	if err != nil {
		t.Fatalf("GenerateMutation failed: %v", err)
	}

	// Two arithmetic expr lines exist, but one is ignored for arithmetic => 4 mutations.
	if len(mutations) != 4 {
		t.Fatalf("expected 4 arithmetic mutations (one line ignored), got %d", len(mutations))
	}
}

func makeSourceV2(t *testing.T, path string) m.Source {
	t.Helper()

	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("failed to resolve path: %v", err)
	}

	return m.Source{
		Origin: &m.File{FullPath: m.Path(abs)},
	}
}

func readFileBytes(t *testing.T, path m.Path) []byte {
	t.Helper()

	content, err := os.ReadFile(string(path))
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}

	return content
}

func newTestMutagen() Mutagen {
	return NewMutagen(adapter.NewLocalGoFileAdapter(), adapter.NewLocalSourceFSAdapter())
}

func TestMutagen_StreamMutations_Success(t *testing.T) {
	mg := newTestMutagen()

	sources := make(chan m.Source, 1)
	source := makeSourceV2(t, filepath.Join("..", "..", "examples", "basic", "main.go"))
	sources <- source
	close(sources)

	mutationCh, errCh := mg.StreamMutations(context.Background(), sources, 4, m.MutationArithmetic)

	var mutations []m.Mutation
	for mutation := range mutationCh {
		mutations = append(mutations, mutation)
	}

	// Check for errors
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("StreamMutations failed: %v", err)
		}
	default:
	}

	if len(mutations) != 4 {
		t.Fatalf("expected 4 arithmetic mutations, got %d", len(mutations))
	}

	for _, mutation := range mutations {
		if mutation.Type != m.MutationArithmetic {
			t.Fatalf("expected arithmetic mutation, got %v", mutation.Type)
		}
	}
}

func TestMutagen_StreamMutations_MultipleSources(t *testing.T) {
	mg := newTestMutagen()

	sources := make(chan m.Source, 2)
	source1 := makeSourceV2(t, filepath.Join("..", "..", "examples", "basic", "main.go"))
	source2 := makeSourceV2(t, filepath.Join("..", "..", "examples", "boolean", "main.go"))
	sources <- source1
	sources <- source2
	close(sources)

	mutationCh, errCh := mg.StreamMutations(context.Background(), sources, 4, m.MutationArithmetic, m.MutationBoolean)

	var mutations []m.Mutation
	for mutation := range mutationCh {
		mutations = append(mutations, mutation)
	}

	// Check for errors
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("StreamMutations failed: %v", err)
		}
	default:
	}

	if len(mutations) == 0 {
		t.Fatal("expected mutations from multiple sources")
	}
}

func TestMutagen_StreamMutations_ContextCancelled(t *testing.T) {
	mg := newTestMutagen()

	ctx, cancel := context.WithCancel(context.Background())

	sources := make(chan m.Source, 1)
	source := makeSourceV2(t, filepath.Join("..", "..", "examples", "basic", "main.go"))
	sources <- source
	close(sources)

	// Cancel context immediately
	cancel()

	mutationCh, errCh := mg.StreamMutations(ctx, sources, 4, m.MutationArithmetic)

	var mutations []m.Mutation
	for mutation := range mutationCh {
		mutations = append(mutations, mutation)
	}

	// Context was cancelled, so we might have fewer mutations or an error
	select {
	case err := <-errCh:
		if err != nil && err != context.Canceled {
			t.Fatalf("unexpected error: %v", err)
		}
	default:
	}
}

func TestMutagen_StreamMutations_InvalidSource(t *testing.T) {
	mg := newTestMutagen()

	sources := make(chan m.Source, 1)
	sources <- m.Source{} // Invalid source - no origin
	close(sources)

	mutationCh, errCh := mg.StreamMutations(context.Background(), sources, 4, m.MutationArithmetic)

	var mutations []m.Mutation
	for mutation := range mutationCh {
		mutations = append(mutations, mutation)
	}

	// Should receive an error
	var receivedErr error
	select {
	case err := <-errCh:
		receivedErr = err
	default:
	}

	if receivedErr == nil {
		t.Fatal("expected error for invalid source")
	}

	if len(mutations) != 0 {
		t.Fatalf("expected no mutations for invalid source, got %d", len(mutations))
	}
}

func TestMutagen_StreamMutations_InvalidMutationType(t *testing.T) {
	mg := newTestMutagen()

	sources := make(chan m.Source, 1)
	source := makeSourceV2(t, filepath.Join("..", "..", "examples", "basic", "main.go"))
	sources <- source
	close(sources)

	invalidType := m.MutationType{Name: "invalid", Version: 1}
	mutationCh, errCh := mg.StreamMutations(context.Background(), sources, 4, invalidType)

	var mutations []m.Mutation
	for mutation := range mutationCh {
		mutations = append(mutations, mutation)
	}

	// Should receive an error
	var receivedErr error
	select {
	case err := <-errCh:
		receivedErr = err
	default:
	}

	if receivedErr == nil {
		t.Fatal("expected error for invalid mutation type")
	}

	if len(mutations) != 0 {
		t.Fatalf("expected no mutations for invalid type, got %d", len(mutations))
	}
}

func TestMutagen_StreamMutations_EmptySources(t *testing.T) {
	mg := newTestMutagen()

	sources := make(chan m.Source)
	close(sources) // Close immediately - no sources

	mutationCh, errCh := mg.StreamMutations(context.Background(), sources, 4, m.MutationArithmetic)

	var mutations []m.Mutation
	for mutation := range mutationCh {
		mutations = append(mutations, mutation)
	}

	// Check for errors
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	default:
	}

	if len(mutations) != 0 {
		t.Fatalf("expected no mutations for empty sources, got %d", len(mutations))
	}
}

func TestMutagen_StreamMutations_DefaultMutationTypes(t *testing.T) {
	mg := newTestMutagen()

	sources := make(chan m.Source, 1)
	source := makeSourceV2(t, filepath.Join("..", "..", "examples", "basic", "main.go"))
	sources <- source
	close(sources)

	// No mutation types specified - should use defaults
	mutationCh, errCh := mg.StreamMutations(context.Background(), sources, 4)

	var mutations []m.Mutation
	for mutation := range mutationCh {
		mutations = append(mutations, mutation)
	}

	// Check for errors
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("StreamMutations failed: %v", err)
		}
	default:
	}

	if len(mutations) == 0 {
		t.Fatal("expected mutations with default types")
	}
}

func TestMutagen_StreamMutations_ThreadsZero(t *testing.T) {
	mg := newTestMutagen()

	sources := make(chan m.Source, 1)
	source := makeSourceV2(t, filepath.Join("..", "..", "examples", "basic", "main.go"))
	sources <- source
	close(sources)

	// threads=0 should normalize to 1
	mutationCh, errCh := mg.StreamMutations(context.Background(), sources, 0, m.MutationArithmetic)

	var mutations []m.Mutation
	for mutation := range mutationCh {
		mutations = append(mutations, mutation)
	}

	// Check for errors
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("StreamMutations failed: %v", err)
		}
	default:
	}

	if len(mutations) != 4 {
		t.Fatalf("expected 4 arithmetic mutations, got %d", len(mutations))
	}
}
