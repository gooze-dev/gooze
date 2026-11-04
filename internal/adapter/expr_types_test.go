package adapter

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gooze.dev/pkg/gooze/internal/domain"
)

// newTestExprTypes returns an ExprTypes that builds export data in a cache
// of the test's own: in the shared GOCACHE, its builds would show up in
// gocache's tests, which check that gooze leaves it alone.
func newTestExprTypes(t *testing.T) *ExprTypes {
	t.Helper()

	types := NewExprTypes()
	types.SetBuildEnv([]string{"GOCACHE=" + t.TempDir()})

	return types
}

// writeModule writes files into a new module and returns its directory.
func writeModule(t *testing.T, files map[string]string) string {
	t.Helper()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/p\n\ngo 1.24\n"), 0o600))

	for name, src := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(src), 0o600))
	}

	return dir
}

// spanOf returns the span of the first occurrence of expr in src.
func spanOf(t *testing.T, src, expr string) domain.Span {
	t.Helper()

	start := strings.Index(src, expr)
	require.GreaterOrEqual(t, start, 0, expr)

	return domain.Span{Start: start, End: start + len(expr)}
}

// spanIn returns the span of expr inside the first occurrence of around in src.
func spanIn(t *testing.T, src, around, expr string) domain.Span {
	t.Helper()

	outer := spanOf(t, src, around)
	start := outer.Start + strings.Index(around, expr)

	return domain.Span{Start: start, End: start + len(expr)}
}

func TestExprTypes_StringExprs(t *testing.T) {
	const a = `package p

import "strconv"

func Name(s string, n int) string {
	return s + strconv.Itoa(n) + suffix
}

type label string

func Label(l label, n int) label {
	return l + label(strconv.Itoa(n+1))
}
`

	const b = "package p\n\nconst suffix = \"!\"\n"

	const test = "package p\n\nfunc helper() string { return \"t\" }\n"

	dir := writeModule(t, map[string]string{"a.go": a, "b.go": b, "a_test.go": test})

	types := newTestExprTypes(t)
	strs := types.StringExprs(context.Background(), filepath.Join(dir, "a.go"))

	for _, expr := range []string{"s + strconv.Itoa(n)", "strconv.Itoa(n)", "suffix", "l + label(strconv.Itoa(n+1))", "label(strconv.Itoa(n+1))"} {
		assert.True(t, strs[spanOf(t, a, expr)], "%q is a string", expr)
	}

	assert.True(t, strs[spanIn(t, a, "return l +", "l")], "l, of a type whose underlying type is string")
	assert.False(t, strs[spanIn(t, a, "Itoa(n)", "n")], "n is an int")
	assert.False(t, strs[spanIn(t, a, "Itoa(n+1)", "n+1")], "n+1 is an int")

	assert.True(t, types.StringExprs(context.Background(), filepath.Join(dir, "b.go"))[spanOf(t, b, `"!"`)],
		"the other files of the package are checked once with it")
	assert.Empty(t, types.StringExprs(context.Background(), filepath.Join(dir, "a_test.go")), "test files are not checked")
}

func TestExprTypes_UnknownTypes(t *testing.T) {
	broken := "package p\n\nfunc F(s string) string {\n\treturn s + missing()\n}\n"
	dir := writeModule(t, map[string]string{"a.go": broken})

	strs := newTestExprTypes(t).StringExprs(context.Background(), filepath.Join(dir, "a.go"))
	assert.True(t, strs[spanIn(t, broken, "return s", "s")], "what type-checks is known")
	assert.False(t, strs[spanOf(t, broken, "missing()")], "an expression that doesn't type-check is unknown")

	assert.Empty(t, newTestExprTypes(t).StringExprs(context.Background(), filepath.Join(t.TempDir(), "gone.go")), "no package")
}

func TestExprTypes_OnlyThePackagesOwnBuiltFiles(t *testing.T) {
	const (
		a       = "package p\n\nvar A = \"a\"\n"
		other   = "package aa\n\nvar B = \"b\"\n" // another package, sorting before p
		last    = "package zz\n\nvar C = \"c\"\n" // another package, sorting after p
		broken  = "package p\n\nvar D = \"d\"\n\nfunc (\n"
		ignored = "//go:build ignore\n\npackage p\n\nvar E = \"e\"\n"
	)

	dir := writeModule(t, map[string]string{"a.go": a, "b.go": other, "c.go": last, "d.go": broken, "e.go": ignored})
	types := newTestExprTypes(t)

	assert.True(t, types.StringExprs(context.Background(), filepath.Join(dir, "a.go"))[spanOf(t, a, `"a"`)])

	for name, src := range map[string]string{"b.go": other, "c.go": last, "d.go": broken, "e.go": ignored} {
		assert.Empty(t, types.StringExprs(context.Background(), filepath.Join(dir, name)), "%s: %q", name, src)
	}
}

func TestExprTypes_UntypedNonStrings(t *testing.T) {
	const src = "package p\n\nvar n = 1\n\nvar e error = nil\n\nvar r = 'r'\n\nvar s = \"s\"\n"

	strs := newTestExprTypes(t).StringExprs(context.Background(), filepath.Join(writeModule(t, map[string]string{"p.go": src}), "p.go"))

	assert.True(t, strs[spanOf(t, src, `"s"`)])

	for _, expr := range []string{"1", "nil", "'r'"} {
		assert.False(t, strs[spanOf(t, src, expr)], "%s is not a string", expr)
	}
}

func TestExprTypes_BuildsInTheBuildEnv(t *testing.T) {
	shared, own := t.TempDir(), t.TempDir()
	t.Setenv("GOCACHE", shared)

	const src = "package p\n\nimport \"strconv\"\n\nfunc F(n int) string { return strconv.Itoa(n) }\n"

	types := NewExprTypes()
	types.SetBuildEnv([]string{"GOCACHE=" + own})

	strs := types.StringExprs(context.Background(), filepath.Join(writeModule(t, map[string]string{"p.go": src}), "p.go"))
	assert.True(t, strs[spanOf(t, src, "strconv.Itoa(n)")], "strconv's export data was built")

	ownEntries, err := os.ReadDir(own)
	require.NoError(t, err)
	assert.NotEmpty(t, ownEntries, "go list built in the cache of the build env")

	sharedEntries, err := os.ReadDir(shared)
	require.NoError(t, err)
	assert.Empty(t, sharedEntries, "and left the shared GOCACHE alone")
}

func TestExprTypes_IndexedExprs(t *testing.T) {
	const src = `package p

type names []string

func F(s []int, a [2]int, pa *[2]int, str string, n int, ns names, m map[int]int, c chan int, f func(func(int) bool), ps *[]int, b bool, x float64) {
	_ = s
	_ = a
	_ = pa
	_ = str
	_ = n
	_ = ns
	_ = m
	_ = c
	_ = f
	_ = ps
	_ = b
	_ = x
}
`

	indexed := newTestExprTypes(t).IndexedExprs(context.Background(), filepath.Join(writeModule(t, map[string]string{"p.go": src}), "p.go"))

	for _, name := range []string{"s", "a", "pa", "str", "n", "ns"} {
		assert.True(t, indexed[spanIn(t, src, "_ = "+name+"\n", name)], "%s is indexed", name)
	}

	for _, name := range []string{"m", "c", "f", "ps", "b", "x"} {
		assert.False(t, indexed[spanIn(t, src, "_ = "+name+"\n", name)], "%s is not", name)
	}
}

func TestExprTypes_IndexedBasicKinds(t *testing.T) {
	names := []string{"i8", "i16", "i32", "i64", "u", "u8", "u16", "u32", "u64", "up", "r"}
	others := []string{"1", "'r'", `"s"`, "1.5", "true"}

	src := "package p\n\nfunc F(i8 int8, i16 int16, i32 int32, i64 int64, u uint, u8 uint8, u16 uint16, u32 uint32, u64 uint64, up uintptr, r rune) {\n"
	for _, expr := range append(slices.Clone(names), others...) {
		src += "\t_ = " + expr + "\n"
	}

	src += "}\n"

	indexed := newTestExprTypes(t).IndexedExprs(context.Background(), filepath.Join(writeModule(t, map[string]string{"p.go": src}), "p.go"))

	for _, expr := range append(names, others[:3]...) {
		assert.True(t, indexed[spanIn(t, src, "_ = "+expr+"\n", expr)], "%s is indexed", expr)
	}

	for _, expr := range others[3:] {
		assert.False(t, indexed[spanIn(t, src, "_ = "+expr+"\n", expr)], "%s is not", expr)
	}
}
