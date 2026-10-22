package domain_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"gooze.dev/pkg/gooze/internal/adapter"
	domain "gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

func TestMutagen_GenerateMutation_ArithmeticBasic(t *testing.T) {
	mg := newTestMutagen()

	source := makeSourceV2(t, filepath.Join("..", "..", "examples", "basic", "main.go"))
	original := readFileBytes(t, m.Path(source.Origin.FullPath))

	mutations, err := mg.GenerateMutation(context.Background(), source, mgen.Arithmetic)
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
		if mutation.Type != mgen.Arithmetic {
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
	original := readFileBytes(t, m.Path(source.Origin.FullPath))

	mutations, err := mg.GenerateMutation(context.Background(), source, mgen.Boolean)
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
		if mutation.Type != mgen.Boolean {
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
	_, err := mg.GenerateMutation(context.Background(), source, mgen.Type{Name: "invalid", Version: 1})
	if err == nil {
		t.Fatalf("expected error for invalid mutation type")
	}
}

func TestMutagen_GenerateMutation_InvalidSource(t *testing.T) {
	mg := newTestMutagen()

	_, err := mg.GenerateMutation(context.Background(), rpt.Source{}, mgen.Arithmetic)
	if err == nil {
		t.Fatalf("expected error for missing source origin")
	}
}

func TestMutagen_GenerateMutation_Ignore_FileLevel_ByMutagenName(t *testing.T) {
	mg := newTestMutagen()

	source := makeSourceV2(t, filepath.Join("..", "..", "examples", "ignore", "file_ignore.go"))

	mutationsArithmetic, err := mg.GenerateMutation(context.Background(), source, mgen.Arithmetic)
	if err != nil {
		t.Fatalf("GenerateMutation(arithmetic) failed: %v", err)
	}
	if len(mutationsArithmetic) != 0 {
		t.Fatalf("expected 0 arithmetic mutations due to file-level ignore, got %d", len(mutationsArithmetic))
	}

	mutationsNumbers, err := mg.GenerateMutation(context.Background(), source, mgen.Numbers)
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

	mutations, err := mg.GenerateMutation(context.Background(), source, mgen.Arithmetic)
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

	mutations, err := mg.GenerateMutation(context.Background(), source, mgen.Arithmetic)
	if err != nil {
		t.Fatalf("GenerateMutation failed: %v", err)
	}

	// Two arithmetic expr lines exist, but one is ignored for arithmetic => 4 mutations.
	if len(mutations) != 4 {
		t.Fatalf("expected 4 arithmetic mutations (one line ignored), got %d", len(mutations))
	}
}

func makeSourceV2(t *testing.T, path string) rpt.Source {
	t.Helper()

	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("failed to resolve path: %v", err)
	}

	return rpt.Source{
		Origin: &rpt.File{FullPath: abs},
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

func newTestMutagen() domain.Mutagen {
	return domain.NewMutagen(adapter.NewLocalGoFileAdapter(), adapter.NewLocalSourceFSAdapter(), adapter.NewUnifiedDiffer())
}

func TestMutagen_AttributesLineNumbers(t *testing.T) {
	mg := newTestMutagen()

	source := makeSourceV2(t, filepath.Join("..", "..", "examples", "boolean", "main.go"))

	mutations, err := mg.GenerateMutation(context.Background(), source, mgen.Boolean)
	require.NoError(t, err)
	require.Len(t, mutations, 4)

	lines := make([]int, 0, len(mutations))
	for _, mut := range mutations {
		require.Positive(t, mut.Line, "mutation should have a 1-based line number")
		lines = append(lines, mut.Line)
	}
	sort.Ints(lines)

	// true@7, false@8, and true/false@18 in examples/boolean/main.go.
	require.Equal(t, []int{7, 8, 18, 18}, lines)
}

type countingDiffer struct{ calls int }

func (d *countingDiffer) Diff(original, mutated []byte) []byte {
	d.calls++
	return adapter.NewUnifiedDiffer().Diff(original, mutated)
}

func TestMutagen_StreamMutations_BuildsMutationsLazily(t *testing.T) {
	differ := &countingDiffer{}
	mg := domain.NewMutagen(adapter.NewLocalGoFileAdapter(), adapter.NewLocalSourceFSAdapter(), differ)
	source := makeSourceV2(t, filepath.Join("..", "..", "examples", "basic", "main.go"))

	all, err := newTestMutagen().GenerateMutation(context.Background(), source, mgen.Arithmetic)
	require.NoError(t, err)
	require.Greater(t, len(all), 1, "fixture must produce several arithmetic mutations")

	stop := errors.New("stop")
	delivered := 0

	err = mg.StreamMutations(context.Background(), source, domain.MutationOptions{}, func(m.Mutation) error {
		delivered++
		return stop
	}, mgen.Arithmetic)

	require.ErrorIs(t, err, stop)
	require.Equal(t, 1, delivered)
	require.Equal(t, 1, differ.calls, "diffs must only be computed for mutations handed to the caller")
}

func streamTypesFromSource(t *testing.T, src string, mutationType mgen.Type) []m.Mutation {
	t.Helper()

	path := filepath.Join(t.TempDir(), "p.go")
	require.NoError(t, os.WriteFile(path, []byte(src), 0o600))

	mutations, err := newTestMutagen().GenerateMutation(context.Background(), makeSourceV2(t, path), mutationType)
	require.NoError(t, err)

	return mutations
}

func linesOf(mutations []m.Mutation) []int {
	lines := make([]int, 0, len(mutations))
	for _, mutation := range mutations {
		lines = append(lines, mutation.Line)
	}

	return lines
}

func TestMutagen_IgnoreDirectiveForOtherMutatorDoesNotSkip(t *testing.T) {
	const src = "package p\n\n" +
		"//gooze:ignore numbers\n" +
		"func f(a int) int {\n" +
		"\treturn a + a\n" +
		"}\n\n" +
		"func g(a int) int {\n" +
		"\treturn a + a //gooze:ignore numbers\n" +
		"}\n"

	lines := linesOf(streamTypesFromSource(t, src, mgen.Arithmetic))
	require.Contains(t, lines, 5, "a function ignore for another mutator must not hide arithmetic mutations")
	require.Contains(t, lines, 9, "a line ignore for another mutator must not hide arithmetic mutations")
}

func TestMutagen_LineIgnoreStillVisitsNestedLines(t *testing.T) {
	const src = "package p\n\n" +
		"func f(a int) int {\n" +
		"\treturn func() int { //gooze:ignore arithmetic\n" +
		"\t\treturn a + a\n" +
		"\t}()\n" +
		"}\n"

	require.Contains(t, linesOf(streamTypesFromSource(t, src, mgen.Arithmetic)), 5,
		"a line ignore must not hide mutations on the following lines of a nested node")
}

func TestMutagen_LineIgnoreHonoursLineDirectives(t *testing.T) {
	const src = "package p\n\n" +
		"//line other.go:100\n" +
		"func f(a int) int {\n" +
		"\treturn a + a //gooze:ignore arithmetic\n" +
		"}\n"

	require.Empty(t, streamTypesFromSource(t, src, mgen.Arithmetic))
}

func TestMutagen_LineIgnoreSkipsBlockStatementMutagens(t *testing.T) {
	// increment and assignment visit a block and change one of its
	// statements, so the ignore must be checked on the changed line.
	const src = "package p\n\n" +
		"func f(n int) int {\n" +
		"\tc := 0\n" +
		"\tc++ //gooze:ignore increment\n" +
		"\tc-- //gooze:ignore\n" +
		"\tc++\n" +
		"\tc += n //gooze:ignore assignment\n" +
		"\tc -= n\n" +
		"\treturn c\n" +
		"}\n"

	require.Equal(t, []int{7}, linesOf(streamTypesFromSource(t, src, mgen.Increment)),
		"only the c++ without a directive is mutated")
	require.Equal(t, []int{9, 9}, linesOf(streamTypesFromSource(t, src, mgen.Assignment)),
		"only c -= n is mutated, to c += n and c = n")
}

func TestMutagen_LineIgnoreSkipsBlockStatementMutagensWithLineDirectives(t *testing.T) {
	const src = "package p\n\n" +
		"//line other.go:100\n" +
		"func f(c int) int {\n" +
		"\tc++ //gooze:ignore increment\n" +
		"\treturn c\n" +
		"}\n"

	require.Empty(t, streamTypesFromSource(t, src, mgen.Increment))
}

func TestMutagen_GenerateMutation_SkipsTypeConstraints(t *testing.T) {
	path := filepath.Join(t.TempDir(), "generic.go")
	code := `package generic

type Number interface {
	~int | ~uint
}

type Set[T int | string] map[T]bool

func Or[T Number | ~int8](a, b T) T {
	return a | b
}
`
	require.NoError(t, os.WriteFile(path, []byte(code), 0o600))

	mutations, err := newTestMutagen().GenerateMutation(context.Background(), makeSourceV2(t, path), mgen.Bitwise)
	require.NoError(t, err)
	require.Len(t, mutations, 1, "only the expression a | b is mutated, not the constraints")
	require.Contains(t, string(mutations[0].MutatedCode), "return a & b")
}
