package adapter

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gooze.dev/pkg/gooze/internal/domain"
)

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

	types := NewExprTypes()
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

	strs := NewExprTypes().StringExprs(context.Background(), filepath.Join(dir, "a.go"))
	assert.True(t, strs[spanIn(t, broken, "return s", "s")], "what type-checks is known")
	assert.False(t, strs[spanOf(t, broken, "missing()")], "an expression that doesn't type-check is unknown")

	assert.Empty(t, NewExprTypes().StringExprs(context.Background(), filepath.Join(t.TempDir(), "gone.go")), "no package")
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
	types := NewExprTypes()

	assert.True(t, types.StringExprs(context.Background(), filepath.Join(dir, "a.go"))[spanOf(t, a, `"a"`)])

	for name, src := range map[string]string{"b.go": other, "c.go": last, "d.go": broken, "e.go": ignored} {
		assert.Empty(t, types.StringExprs(context.Background(), filepath.Join(dir, name)), "%s: %q", name, src)
	}
}

func TestExprTypes_UntypedNonStrings(t *testing.T) {
	const src = "package p\n\nvar n = 1\n\nvar e error = nil\n\nvar r = 'r'\n\nvar s = \"s\"\n"

	strs := NewExprTypes().StringExprs(context.Background(), filepath.Join(writeModule(t, map[string]string{"p.go": src}), "p.go"))

	assert.True(t, strs[spanOf(t, src, `"s"`)])

	for _, expr := range []string{"1", "nil", "'r'"} {
		assert.False(t, strs[spanOf(t, src, expr)], "%s is not a string", expr)
	}
}
