package domain_test

import (
	"context"
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gooze.dev/pkg/gooze/internal/adapter"
	domain "gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
)

// enumMutants writes files to a directory and returns the changed lines of the
// comparison mutants of its p.go, sorted, with run.enum_bounds set as given.
func enumMutants(t *testing.T, dir string, enumBounds bool) []string {
	t.Helper()

	var mutations []m.Mutation

	err := newTestMutagen().StreamMutations(context.Background(), makeSourceV2(t, filepath.Join(dir, "p.go")),
		domain.MutationOptions{EnumBounds: enumBounds}, func(mutation m.Mutation) error {
			mutations = append(mutations, mutation)
			return nil
		}, mgen.Comparison)
	require.NoError(t, err)

	return mutatedLineTexts(mutations)
}

const kinds = "package p\n\ntype Kind int\n\nconst (\n\tKindA Kind = iota\n\tKindB\n\tKindC\n)\n\n"

func TestMutagen_EnumBounds(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string // p.go is mutated
		skip  []string          // the changed lines of the skipped mutants, sorted
	}{
		{name: "== lowest", files: map[string]string{"p.go": kinds + "func f(k Kind) bool {\n\treturn k == KindA\n}\n"}, skip: []string{"return k <= KindA"}},
		{name: "!= lowest", files: map[string]string{"p.go": kinds + "func f(k Kind) bool {\n\treturn k != KindA\n}\n"}, skip: []string{"return k > KindA"}},
		{name: "== highest", files: map[string]string{"p.go": kinds + "func f(k Kind) bool {\n\treturn k == KindC\n}\n"}, skip: []string{"return k >= KindC"}},
		{name: "!= highest", files: map[string]string{"p.go": kinds + "func f(k Kind) bool {\n\treturn k != KindC\n}\n"}, skip: []string{"return k < KindC"}},
		{name: "lowest ==", files: map[string]string{"p.go": kinds + "func f(k Kind) bool {\n\treturn KindA == k\n}\n"}, skip: []string{"return KindA >= k"}},
		{name: "lowest !=", files: map[string]string{"p.go": kinds + "func f(k Kind) bool {\n\treturn KindA != k\n}\n"}, skip: []string{"return KindA < k"}},
		{name: "highest ==", files: map[string]string{"p.go": kinds + "func f(k Kind) bool {\n\treturn KindC == k\n}\n"}, skip: []string{"return KindC <= k"}},
		{name: "highest !=", files: map[string]string{"p.go": kinds + "func f(k Kind) bool {\n\treturn KindC != k\n}\n"}, skip: []string{"return KindC > k"}},
		{name: "middle constant", files: map[string]string{"p.go": kinds + "func f(k Kind) bool {\n\treturn k == KindB\n}\n"}},
		{name: "ordering comparison", files: map[string]string{"p.go": kinds + "func f(k Kind) bool {\n\treturn k < KindC\n}\n"}},
		{
			name:  "field and call",
			files: map[string]string{"p.go": kinds + "type s struct{ k Kind }\n\nfunc (v s) kind() Kind { return v.k }\n\nfunc f(v s) bool {\n\treturn v.k == KindA || v.kind() == KindC\n}\n"},
			skip:  []string{"return v.k <= KindA || v.kind() == KindC", "return v.k == KindA || v.kind() >= KindC"},
		},
		{
			name:  "constants in another file",
			files: map[string]string{"p.go": "package p\n\nfunc f(k Kind) bool {\n\treturn k == KindC\n}\n", "kinds.go": kinds},
			skip:  []string{"return k >= KindC"},
		},
		{
			name:  "a constant in an internal test file",
			files: map[string]string{"p.go": kinds + "func f(k Kind) bool {\n\treturn k == KindC\n}\n", "p_test.go": "package p\n\nconst kindD Kind = 3\n"},
		},
		{
			name:  "an external test file",
			files: map[string]string{"p.go": kinds + "func f(k Kind) bool {\n\treturn k == KindC\n}\n", "p_test.go": "package p_test\n\nconst kindD = 3\n\ntype Kind int\n\nconst KindZ Kind = 9\n"},
			skip:  []string{"return k >= KindC"},
		},
		{
			name:  "a file of the package that doesn't parse",
			files: map[string]string{"p.go": kinds + "func f(k Kind) bool {\n\treturn k == KindC\n}\n", "broken.go": "package p\n\nfunc (\n"},
		},
		{
			name:  "a directory next to the file",
			files: map[string]string{"p.go": kinds + "func f(k Kind) bool {\n\treturn k == KindC\n}\n", "sub/q.go": "package p\n\nconst KindD Kind = 3\n"},
			skip:  []string{"return k >= KindC"},
		},
		{
			name:  "a file that isn't Go",
			files: map[string]string{"p.go": kinds + "func f(k Kind) bool {\n\treturn k == KindC\n}\n", "notes.txt": "package p\n\nconst KindD Kind = 3\n"},
			skip:  []string{"return k >= KindC"},
		},
		{name: "a literal operand", files: map[string]string{"p.go": kinds + "func f() bool {\n\treturn 0 == KindA\n}\n"}},
		{name: "a constant operand", files: map[string]string{"p.go": kinds + "func f() bool {\n\treturn KindB == KindA\n}\n"}},
		{name: "a local constant operand", files: map[string]string{"p.go": kinds + "func f() bool {\n\tconst x = 2\n\treturn x == KindA\n}\n"}},
		{name: "a parameter shadowing the constant", files: map[string]string{"p.go": kinds + "func f(k, KindA Kind) bool {\n\treturn k == KindA\n}\n"}},
		{name: "an indexed operand", files: map[string]string{"p.go": kinds + "func f(k []Kind) bool {\n\treturn k[0] == KindA\n}\n"}},
		{name: "a single constant", files: map[string]string{"p.go": "package p\n\ntype Kind int\n\nconst KindA Kind = 0\n\nfunc f(k Kind) bool {\n\treturn k == KindA\n}\n"}},
		{name: "a string type", files: map[string]string{"p.go": "package p\n\ntype Kind string\n\nconst (\n\tKindA Kind = \"a\"\n\tKindB Kind = \"b\"\n)\n\nfunc f(k Kind) bool {\n\treturn k == KindA\n}\n"}},
		{name: "a type defined from another", files: map[string]string{"p.go": "package p\n\ntype Base int\n\ntype Kind Base\n\nconst (\n\tKindA Kind = iota\n\tKindB\n)\n\nfunc f(k Kind) bool {\n\treturn k == KindA\n}\n"}},
		{name: "an alias", files: map[string]string{"p.go": "package p\n\ntype Kind = int\n\nconst (\n\tKindA Kind = iota\n\tKindB\n)\n\nfunc f(k Kind) bool {\n\treturn k == KindA\n}\n"}},
		{name: "a generic type", files: map[string]string{"p.go": "package p\n\ntype Kind[T any] int\n\nfunc f(k Kind[int]) bool {\n\treturn k == 0\n}\n"}},
		{name: "a type declared in a function", files: map[string]string{"p.go": "package p\n\nfunc f(k int) bool {\n\ttype Kind int\n\n\tconst (\n\t\tKindA Kind = iota\n\t\tKindB\n\t)\n\n\treturn Kind(k) == KindA\n}\n"}},
		{name: "a conversion", files: map[string]string{"p.go": kinds + "func f(n int) bool {\n\treturn Kind(n) == KindA\n}\n"}},
		{name: "a conversion elsewhere", files: map[string]string{"p.go": kinds + "func f(k Kind) bool {\n\treturn k == KindA\n}\n", "q.go": "package p\n\nvar k = (Kind)(7)\n"}},
		{name: "arithmetic on a constant", files: map[string]string{"p.go": kinds + "func f(k Kind) bool {\n\treturn k == KindA\n}\n\nvar next = KindC + 1\n"}},
		{name: "arithmetic on a parenthesized constant", files: map[string]string{"p.go": kinds + "func f(k Kind) bool {\n\treturn k == KindA\n}\n\nvar prev = 1 - (KindA)\n"}},
		{
			name:  "logical and comparison operators",
			files: map[string]string{"p.go": kinds + "func f(k Kind, ok bool) bool {\n\treturn ok && k == KindA || k >= KindC\n}\n"},
			skip:  []string{"return ok && k <= KindA || k >= KindC"},
		},
		{name: "an untyped constant from a typed one", files: map[string]string{"p.go": kinds + "const alias = KindC\n\nfunc f(k Kind) bool {\n\treturn k == KindA\n}\n"}},
		{name: "a constant this can't evaluate", files: map[string]string{"p.go": "package p\n\ntype Kind int\n\nconst (\n\tKindA Kind = iota\n\tKindB\n\tKindC Kind = len(\"ab\") * 2\n\tKindD Kind = 2 * len(\"ab\")\n)\n\nfunc f(k Kind) bool {\n\treturn k == KindB\n}\n"}},
		{name: "a rune literal", files: map[string]string{"p.go": "package p\n\ntype Kind rune\n\nconst (\n\tKindA Kind = 0\n\tKindB Kind = 'b'\n)\n\nfunc f(k Kind) bool {\n\treturn k == KindA\n}\n"}, skip: []string{"return k <= KindA"}},
		{
			name:  "the zero value below the constants",
			files: map[string]string{"p.go": "package p\n\ntype Kind int\n\nconst (\n\tKindA Kind = iota + 1\n\tKindB\n\tKindC\n)\n\nfunc f(k Kind) bool {\n\treturn k == KindA || k == KindC\n}\n"},
			skip:  []string{"return k == KindA || k >= KindC"},
		},
		{
			name:  "a blank constant",
			files: map[string]string{"p.go": "package p\n\ntype Kind uint8\n\nconst (\n\t_ Kind = iota + 1\n\tKindA\n\tKindB\n)\n\nfunc f(k Kind) bool {\n\treturn k == KindA || k == KindB\n}\n"},
			skip:  []string{"return k == KindA || k >= KindB"},
		},
		{
			name:  "explicit values",
			files: map[string]string{"p.go": "package p\n\ntype Kind int64\n\nconst (\n\tKindA Kind = 0x10\n\tKindB Kind = -(2 * 3)\n\tKindC Kind = +4 - 1\n)\n\nfunc f(k Kind) bool {\n\treturn k == KindA || k == KindB || k == KindC\n}\n"},
			skip:  []string{"return k == KindA || k <= KindB || k == KindC", "return k >= KindA || k == KindB || k == KindC"},
		},
		{
			name:  "flags",
			files: map[string]string{"p.go": "package p\n\ntype Kind uint\n\nconst (\n\tKindA Kind = 1 << iota\n\tKindB\n\tKindC\n)\n\nfunc f(k Kind) bool {\n\treturn k == KindA || k == KindC\n}\n"},
			skip:  []string{"return k == KindA || k >= KindC"},
		},
		{name: "a negative shift", files: map[string]string{"p.go": "package p\n\ntype Kind int\n\nconst (\n\tKindA Kind = 1 << -1\n\tKindB Kind = 0\n\tKindC Kind = 1\n)\n\nfunc f(k Kind) bool {\n\treturn k == KindC\n}\n"}},
		{name: "a wide shift", files: map[string]string{"p.go": "package p\n\ntype Kind int\n\nconst (\n\tKindA Kind = 1 << 63\n\tKindB Kind = 0\n\tKindC Kind = 1\n)\n\nfunc f(k Kind) bool {\n\treturn k == KindC\n}\n"}},
		{
			name:  "the widest shift",
			files: map[string]string{"p.go": "package p\n\ntype Kind int\n\nconst (\n\tKindA Kind = 1 << 62\n\tKindB Kind = 0\n)\n\nfunc f(k Kind) bool {\n\treturn k == KindB || k == KindA\n}\n"},
			skip:  []string{"return k <= KindB || k == KindA", "return k == KindB || k >= KindA"},
		},
		{name: "another operator", files: map[string]string{"p.go": "package p\n\ntype Kind int\n\nconst (\n\tKindA Kind = 1 / 1\n\tKindB Kind = 0\n\tKindC Kind = 1\n)\n\nfunc f(k Kind) bool {\n\treturn k == KindC\n}\n"}},
		{name: "another unary operator", files: map[string]string{"p.go": "package p\n\ntype Kind int\n\nconst (\n\tKindA Kind = ^1\n\tKindB Kind = 0\n\tKindC Kind = 1\n)\n\nfunc f(k Kind) bool {\n\treturn k == KindC\n}\n"}},
		{
			name:  "a constant shadowing iota",
			files: map[string]string{"p.go": "package p\n\nconst iota = 5\n\ntype Kind int\n\nconst (\n\tKindA Kind = iota\n\tKindB Kind = 1\n\tKindC Kind = 2\n)\n\nfunc f(k Kind) bool {\n\treturn k == KindC\n}\n"},
		},
		{
			name:  "a constant declared without its value",
			files: map[string]string{"p.go": "package p\n\ntype Kind int\n\nconst (\n\tKindA, KindB Kind = 0, 1\n\tKindC, KindD, KindE, KindF\n)\n\nfunc f(k Kind) bool {\n\treturn k == KindB\n}\n"},
			skip:  []string{"return k >= KindB"},
		},
		{
			name:  "an integer type declared in the file",
			files: map[string]string{"p.go": "package p\n\ntype int string\n\ntype Kind int\n\nconst (\n\tKindA Kind = \"\"\n\tKindB Kind = \"b\"\n)\n\nfunc f(k Kind) bool {\n\treturn k == KindA\n}\n"},
		},
		{
			name:  "a byte type",
			files: map[string]string{"p.go": "package p\n\ntype Kind byte\n\nconst (\n\tKindA Kind = iota\n\tKindB\n)\n\nfunc f(k Kind) bool {\n\treturn k == KindA\n}\n"},
			skip:  []string{"return k <= KindA"},
		},
	}

	tests = append(tests, []struct {
		name  string
		files map[string]string
		skip  []string
	}{
		{name: "lowercase constants", files: map[string]string{"p.go": "package p\n\ntype kind int\n\nconst (\n\tkindA kind = iota\n\tkindB\n)\n\nfunc f(k kind) bool {\n\treturn k == kindA\n}\n"}, skip: []string{"return k <= kindA"}},
		{name: "an untyped constant from a parenthesized typed one", files: map[string]string{"p.go": kinds + "const alias = (KindC)\n\nfunc f(k Kind) bool {\n\treturn k == KindA\n}\n"}},
		{name: "negation of a constant", files: map[string]string{"p.go": kinds + "func f(k Kind) bool {\n\treturn k == KindA\n}\n\nvar neg = -KindC\n"}},
		{
			name:  "negative values",
			files: map[string]string{"p.go": "package p\n\ntype Kind int\n\nconst (\n\tKindA Kind = -1\n\tKindB Kind = 0\n)\n\nfunc f(k Kind) bool {\n\treturn k == KindA || k == KindB\n}\n"},
			skip:  []string{"return k <= KindA || k == KindB", "return k == KindA || k >= KindB"},
		},
		{
			name:  "a variable of the type",
			files: map[string]string{"p.go": kinds + "var kindVar Kind = KindA\n\nfunc f(k Kind) bool {\n\treturn k == KindC\n}\n"},
			skip:  []string{"return k >= KindC"},
		},
		{name: "a float type", files: map[string]string{"p.go": "package p\n\ntype Kind float64\n\nconst (\n\tKindA Kind = iota\n\tKindB\n)\n\nfunc f(k Kind) bool {\n\treturn k == KindA\n}\n"}},
		{
			name:  "a difference",
			files: map[string]string{"p.go": "package p\n\ntype Kind int\n\nconst (\n\tKindA Kind = 4 - 10\n\tKindB Kind = -5\n)\n\nfunc f(k Kind) bool {\n\treturn k == KindA || k == KindB\n}\n"},
			skip:  []string{"return k <= KindA || k == KindB"},
		},
		{
			name:  "a product",
			files: map[string]string{"p.go": "package p\n\ntype Kind int\n\nconst (\n\tKindA Kind = 2 * 3\n\tKindB Kind = 5\n)\n\nfunc f(k Kind) bool {\n\treturn k == KindA || k == KindB\n}\n"},
			skip:  []string{"return k >= KindA || k == KindB"},
		},
		{name: "greater than the highest", files: map[string]string{"p.go": kinds + "func f(k Kind) bool {\n\treturn k > KindC || k < KindA || k >= KindC || k <= KindA\n}\n"}},
		{name: "a literal on the right", files: map[string]string{"p.go": kinds + "func f() bool {\n\treturn KindA == 0\n}\n"}},
		{name: "two variables", files: map[string]string{"p.go": kinds + "func f(j, k Kind) bool {\n\treturn j == k\n}\n"}},
		{
			name:  "a directory named like a Go file",
			files: map[string]string{"p.go": kinds + "func f(k Kind) bool {\n\treturn k == KindC\n}\n", "q.go/r.go": "package p\n"},
			skip:  []string{"return k >= KindC"},
		},
		{
			name:  "a file of another package",
			files: map[string]string{"p.go": kinds + "func f(k Kind) bool {\n\treturn k == KindC\n}\n", "a.go": "package a\n\nconst KindD Kind = 3\n"},
			skip:  []string{"return k >= KindC"},
		},
		{
			name:  "a constant operand from another file",
			files: map[string]string{"p.go": kinds + "func f() bool {\n\treturn limit == KindA\n}\n", "q.go": "package p\n\nconst limit = 2\n"},
		},
		{name: "a function operand", files: map[string]string{"p.go": kinds + "func g() Kind { return KindA }\n\nfunc f() bool {\n\treturn g == KindA\n}\n"}},
	}...)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, src := range tt.files {
				require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o750))
				require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(src), 0o600))
			}

			all := enumMutants(t, dir, false)
			kept := enumMutants(t, dir, true)

			skipped := slices.DeleteFunc(slices.Clone(all), func(line string) bool { return slices.Contains(kept, line) })
			if len(skipped) == 0 {
				skipped = nil
			}

			assert.Equal(t, tt.skip, skipped)
			assert.Len(t, kept, len(all)-len(tt.skip))
		})
	}
}

func TestMutagen_EnumBoundsInADirectoryNamedLikeAGoFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "x.go")
	require.NoError(t, os.Mkdir(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "p.go"), []byte(kinds+"func f(k Kind) bool {\n\treturn k == KindC\n}\n"), 0o600))

	assert.NotContains(t, enumMutants(t, dir, true), "return k >= KindC")
	assert.Contains(t, enumMutants(t, dir, false), "return k >= KindC")
}

// countingParser counts the files it parses.
type countingParser struct {
	domain.GoFileAdapter
	parsed int
}

func (p *countingParser) Parse(ctx context.Context, fileSet *token.FileSet, filename string, src []byte) (*ast.File, error) {
	p.parsed++
	return p.GoFileAdapter.Parse(ctx, fileSet, filename, src)
}

func TestMutagen_EnumBoundsCachedPerPackage(t *testing.T) {
	dir := t.TempDir()
	kindsPath := filepath.Join(dir, "kinds.go")
	require.NoError(t, os.WriteFile(kindsPath, []byte(kinds), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "p.go"), []byte("package p\n\nfunc f(k Kind) bool {\n\treturn k == KindC\n}\n"), 0o600))

	parser := &countingParser{GoFileAdapter: adapter.NewLocalGoFileAdapter()}
	mg := domain.NewMutagen(parser, adapter.NewLocalSourceFSAdapter(), adapter.NewUnifiedDiffer())
	source := makeSourceV2(t, filepath.Join(dir, "p.go"))

	stream := func() []string {
		var mutations []m.Mutation

		err := mg.StreamMutations(context.Background(), source, domain.MutationOptions{EnumBounds: true}, func(mutation m.Mutation) error {
			mutations = append(mutations, mutation)
			return nil
		}, mgen.Comparison)
		require.NoError(t, err)

		return mutatedLineTexts(mutations)
	}

	assert.NotContains(t, stream(), "return k >= KindC")
	assert.Equal(t, 2, parser.parsed, "p.go, then kinds.go for the enums")

	assert.NotContains(t, stream(), "return k >= KindC")
	assert.Equal(t, 3, parser.parsed, "only p.go: the enums are cached")

	// KindC is no longer the highest value once the package changes.
	require.NoError(t, os.WriteFile(kindsPath, []byte(kinds+"const KindD Kind = 3\n"), 0o600))
	assert.Contains(t, stream(), "return k >= KindC")
	assert.Equal(t, 5, parser.parsed, "p.go and the changed kinds.go")
}
