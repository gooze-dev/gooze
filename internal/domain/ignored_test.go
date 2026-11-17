package domain_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gooze.dev/pkg/gooze/internal/adapter"
	domain "gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
	m "gooze.dev/pkg/gooze/internal/model"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
)

// ignoredIn returns the mutants of src that its directives ignore, as
// "line mutagen reason".
func ignoredIn(t *testing.T, src string, types ...mgen.Type) []string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "p.go")
	require.NoError(t, os.WriteFile(path, []byte(src), 0o600))

	mg, ok := newTestMutagen().(domain.MutagenWithOptions)
	require.True(t, ok)

	var got []string

	err := mg.StreamMutationsWith(context.Background(), makeSourceV2(t, path), domain.MutationOptions{Ignored: true}, func(mutation m.Mutation) error {
		require.NotNil(t, mutation.Ignored)
		got = append(got, fmt.Sprintf("%d %s %s", mutation.Line, mutation.Type.Name, mutation.Ignored.Reason))

		return nil
	}, types...)
	require.NoError(t, err)

	return got
}

func TestMutagen_IgnoredStreamsOnlyWhatDirectivesIgnore(t *testing.T) {
	const src = `package p

func a(x int) int {
	_ = x + 1 //gooze:ignore arithmetic reason=equivalent
	_ = x + 2
	return x
}
`

	assert.Equal(t, []string{
		"4 arithmetic equivalent", "4 arithmetic equivalent", "4 arithmetic equivalent", "4 arithmetic equivalent",
	}, ignoredIn(t, src, mgen.Arithmetic))
	assert.Empty(t, ignoredIn(t, "package p\n\nfunc a(x int) int { return x + 1 }\n", mgen.Arithmetic), "no directive ignores nothing")

	path := filepath.Join(t.TempDir(), "p.go")
	require.NoError(t, os.WriteFile(path, []byte(src), 0o600))

	kept, err := newTestMutagen().GenerateMutation(context.Background(), makeSourceV2(t, path), mgen.Arithmetic)
	require.NoError(t, err)
	assert.Equal(t, []int{5, 5, 5, 5}, linesOf(kept))

	for _, mutation := range kept {
		assert.Nil(t, mutation.Ignored, "a mutant that is tested isn't ignored")
	}
}

func TestMutagen_IgnoredNamesTheReasonOfTheScope(t *testing.T) {
	const src = `//gooze:ignore boolean reason=not-useful generated flags

package p

//gooze:ignore reason=deferred
func a(x int) bool {
	_ = x + 1
	return true
}

func b(x int) bool {
	_ = x - 1 //gooze:ignore arithmetic(+) reason=tested-elsewhere
	// gooze:ignore arithmetic
	_ = x * 2
	return x > 0 && true //gooze:ignore comparison(>=) reason=equivalent x is positive
}
`

	assert.Equal(t, []string{
		"7 arithmetic deferred", "7 arithmetic deferred", "7 arithmetic deferred", "7 arithmetic deferred",
		"12 arithmetic tested-elsewhere",
		"14 arithmetic ", "14 arithmetic ", "14 arithmetic ", "14 arithmetic ",
	}, ignoredIn(t, src, mgen.Arithmetic), "the function's reason, one replacement's reason, and no reason")

	assert.Equal(t, []string{"8 boolean deferred", "15 boolean not-useful"}, ignoredIn(t, src, mgen.Boolean),
		"the nearest scope gives the reason: the function before the file")
	assert.Equal(t, []string{"15 comparison equivalent"}, ignoredIn(t, src, mgen.Comparison))
}

func TestMutagen_IgnoredKeepsTheFirstReasonOfAScope(t *testing.T) {
	const src = `package p

// a adds.
//gooze:ignore arithmetic reason=equivalent
//gooze:ignore arithmetic reason=deferred
//gooze:ignore numbers
func a(x int) int {
	return x + 1
}
`

	assert.Equal(t, []string{"8 arithmetic equivalent", "8 arithmetic equivalent", "8 arithmetic equivalent", "8 arithmetic equivalent"},
		ignoredIn(t, src, mgen.Arithmetic))

	numbers := ignoredIn(t, src, mgen.Numbers)
	require.NotEmpty(t, numbers)
	assert.Equal(t, "8 numbers ", numbers[0], "a mutagen without a reason doesn't borrow another's")
}

// ignoredModule writes a module with ignored mutants in a.go and sub/b.go and
// returns its directory.
func ignoredModule(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/ignored\n\ngo 1.24\n",
		"a.go": `package ignored

func A(x int) bool {
	_ = x + 1 //gooze:ignore arithmetic(-) reason=equivalent
	return x > 0 //gooze:ignore comparison(>=, !=) reason=not-useful
}
`,
		"sub/b.go": `package sub

func B(x int) int {
	return x * 2 //gooze:ignore arithmetic(/)
}

func C(x int) int {
	return x + 2
}
`,
	}

	for name, content := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}

	return dir
}

func viewIgnored(t *testing.T, args domain.EstimateArgs) string {
	t.Helper()

	var out bytes.Buffer

	wf := domain.NewWorkflow(adapter.NewLocalSourceFSAdapter(), adapter.NewReportStore(), domainmocks.NewMockReporter(t), nil, newTestMutagen())
	require.NoError(t, wf.Ignored(context.Background(), domain.IgnoredArgs{EstimateArgs: args, Output: &out}))

	return out.String()
}

func TestWorkflow_IgnoredListsTheMutantsWithTheirReasons(t *testing.T) {
	t.Chdir(ignoredModule(t))

	assert.Equal(t, `Ignored (4):
a.go:4: arithmetic (equivalent): _ = x + 1 //gooze:ignore arithmetic(-) reason=equivalent -> _ = x - 1 //gooze:ignore arithmetic(-) reason=equivalent
a.go:5: comparison (not-useful): return x > 0 //gooze:ignore comparison(>=, !=) reason=not-useful -> return x >= 0 //gooze:ignore comparison(>=, !=) reason=not-useful
a.go:5: comparison (not-useful): return x > 0 //gooze:ignore comparison(>=, !=) reason=not-useful -> return x != 0 //gooze:ignore comparison(>=, !=) reason=not-useful
sub/b.go:4: arithmetic: return x * 2 //gooze:ignore arithmetic(/) -> return x / 2 //gooze:ignore arithmetic(/)

By reason: not-useful 2, equivalent 1, no reason 1
By mutagen: arithmetic 2, comparison 2
`, viewIgnored(t, domain.EstimateArgs{Paths: []m.Path{"./..."}}))
}

func TestWorkflow_IgnoredFollowsTheSelection(t *testing.T) {
	t.Chdir(ignoredModule(t))

	assert.Equal(t, `Ignored (1):
sub/b.go:4: arithmetic: return x * 2 //gooze:ignore arithmetic(/) -> return x / 2 //gooze:ignore arithmetic(/)

By reason: no reason 1
By mutagen: arithmetic 1
`, viewIgnored(t, domain.EstimateArgs{Paths: []m.Path{"./sub/..."}}), "only the paths asked for")

	assert.Equal(t, `Ignored (2):
a.go:5: comparison (not-useful): return x > 0 //gooze:ignore comparison(>=, !=) reason=not-useful -> return x >= 0 //gooze:ignore comparison(>=, !=) reason=not-useful
a.go:5: comparison (not-useful): return x > 0 //gooze:ignore comparison(>=, !=) reason=not-useful -> return x != 0 //gooze:ignore comparison(>=, !=) reason=not-useful

By reason: not-useful 2
By mutagen: comparison 2
`, viewIgnored(t, domain.EstimateArgs{Paths: []m.Path{"./..."}, Mutagens: []string{"comparison"}}), "only the mutagens asked for")

	assert.Equal(t, "No mutants are ignored.\n", viewIgnored(t, domain.EstimateArgs{Paths: []m.Path{"./..."}, Run: "^C$"}))
}

func TestMutagen_IgnoredSkipsWhatIsNeverMutated(t *testing.T) {
	const generic = `//gooze:ignore

package p

type number interface{ ~int | ~int64 }

func sum[T int | int64](a, b T) T {
	return a
}
`

	assert.Empty(t, ignoredIn(t, generic), "type constraints aren't mutated, so no directive ignores their mutants")

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kinds.go"), []byte(kinds), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "p.go"),
		[]byte("//gooze:ignore comparison\n\npackage p\n\nfunc f(k Kind) bool {\n\treturn k == KindC\n}\n"), 0o600))

	mg, ok := newTestMutagen().(domain.MutagenWithOptions)
	require.True(t, ok)

	ignored := func(enumBounds bool) []string {
		var mutations []m.Mutation

		err := mg.StreamMutationsWith(context.Background(), makeSourceV2(t, filepath.Join(dir, "p.go")),
			domain.MutationOptions{Ignored: true, EnumBounds: enumBounds}, func(mutation m.Mutation) error {
				mutations = append(mutations, mutation)
				return nil
			}, mgen.Comparison)
		require.NoError(t, err)

		return mutatedLineTexts(mutations)
	}

	assert.Contains(t, ignored(false), "return k >= KindC")
	assert.NotContains(t, ignored(true), "return k >= KindC", "an enum bound mutant is left out without a directive")
	assert.Contains(t, ignored(true), "return k != KindC")
}

func TestMutagen_IgnoredHonoursLineDirectives(t *testing.T) {
	const src = `package p

func a(x int) int {
//line gen.go:50
	_ = (x + //gooze:ignore arithmetic reason=equivalent
		1) * 2
	_ = x - 1 //gooze:ignore arithmetic(+) reason=deferred
	return x
}
`

	got := ignoredIn(t, src, mgen.Arithmetic)

	assert.Contains(t, got, "6 arithmetic equivalent", "the directive on the line the expression starts on covers its second line")
	assert.Contains(t, got, "7 arithmetic deferred", "a single replacement on a line that //line renumbers")
	assert.Len(t, got, 9, "four mutants of +, four of *, one of -")
}
