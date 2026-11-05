package domain

import (
	"bytes"
	"go/constant"
	"go/token"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	m "gooze.dev/pkg/gooze/internal/model"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
)

func TestExprValue(t *testing.T) {
	known := map[string]string{
		"10":        "10",
		"0x1":       "1",
		"1.5":       "3/2",
		"'a'":       "97",
		`"s"`:       `"s"`,
		"(2)":       "2",
		"-3":        "-3",
		"+3":        "3",
		"^1":        "-2",
		"2 * 5":     "10",
		"2 + 3":     "5",
		"2 - 3":     "-1",
		"7 / 2":     "3", // integers truncate
		"7.0 / 2":   "7/2",
		"7 % 3":     "1",
		"6 & 3":     "2",
		"6 & 0":     "0",
		"6 | 3":     "7",
		"6 ^ 3":     "5",
		"6 &^ 3":    "4",
		"-1.5":      "-3/2",
		"7 / -1":    "-7",
		"7 % -2":    "1",
		"7 / 2.0":   "7/2",
		`"a" + "b"`: `"ab"`,
		"1i":        "(0 + 1i)",
	}

	for src, want := range known {
		v, ok := exprValue([]byte(src))
		if assert.True(t, ok, src) {
			assert.Equal(t, want, v.ExactString(), src)
		}
	}

	unknown := []string{
		"x", "-x", "^1.0", "!1", "x + 1", "1 + x", "7 / 0", "7.0 / 0", "7 % 0", "7.0 % 2", "7 % 2.0",
		"6.0 & 1", "6 & 1.0", "1 << 2", "1 == 1", "f(1)", "1 +",
		`-"s"`, `^"s"`, "!1", "!1.5", "&1", `"a" / 2`, `"a" - "b"`, `1 + "a"`, `"a" + 1`, `"a" % 2`, `"a" & 1`, "7.0 % 2.0",
	}

	for _, src := range unknown {
		_, ok := exprValue([]byte(src))
		assert.False(t, ok, src)
	}
}

func TestEqualValues(t *testing.T) {
	i := func(n int64) constant.Value { return constant.MakeInt64(n) }
	f := func(x float64) constant.Value { return constant.MakeFloat64(x) }
	s := constant.MakeString

	assert.True(t, equalValues(i(1), f(1)))
	assert.True(t, equalValues(f(1), i(1)))
	assert.True(t, equalValues(f(1.5), f(1.5)))
	assert.True(t, equalValues(s("a"), s("a")))
	assert.False(t, equalValues(i(1), i(2)))
	assert.False(t, equalValues(f(1.5), f(2.5)))
	assert.False(t, equalValues(i(1), s("1")))
	assert.False(t, equalValues(s("1"), i(1)))
	assert.False(t, equalValues(f(1), s("1")))
	assert.False(t, equalValues(s("1"), f(1)))
	assert.False(t, equalValues(s("a"), s("b")))
	assert.True(t, equalValues(constant.MakeImag(i(0)), i(0)), "a complex number is a number")
}

func TestIsZero(t *testing.T) {
	assert.True(t, isZero(constant.MakeInt64(0), true))
	assert.False(t, isZero(constant.MakeInt64(0), false))
	assert.False(t, isZero(constant.MakeInt64(1), true))
	assert.False(t, isZero(constant.MakeInt64(-1), true))
}

func TestSpan_Contains(t *testing.T) {
	s := Span{Start: 2, End: 5}

	assert.False(t, s.contains(1))
	assert.True(t, s.contains(2))
	assert.True(t, s.contains(4))
	assert.False(t, s.contains(5))
}

func TestDuplicatesCase(t *testing.T) {
	content := []byte("7,1")
	values := []Span{{Start: 0, End: 1}, {Start: 2, End: 3}}

	cases := map[string]bool{
		"1,1":  true,  // the first value made equal to the second
		"7,7":  true,  // the second made equal to the first
		"3,1":  false, // a new value
		"07,1": false, // the same value as before
		"x,1":  false, // unknown
	}

	for mutated, want := range cases {
		at := firstDifference(content, []byte(mutated))
		assert.Equal(t, want, duplicatesCase(values, at, content, []byte(mutated)), mutated)
	}

	assert.False(t, duplicatesCase(values, 1, []byte("7;1"), []byte("7:1")), "a change outside the values")
}

func TestBuildChecks_BreaksBuild(t *testing.T) {
	c := &buildChecks{unordered: map[int]bool{0: true}}

	assert.True(t, c.breaksBuild([]byte("== b"), []byte("< b")), "an ordering operator at the start")
	assert.False(t, c.breaksBuild([]byte("== b"), []byte("!= b")), "an equality operator")
	assert.False(t, c.breaksBuild([]byte("== b"), []byte("== b")), "no change")
}

func TestBuildChecks_CountsAndLogsSkipped(t *testing.T) {
	var logs bytes.Buffer

	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	content := []byte("x / 2")
	c := &buildChecks{divisors: []Span{{Start: 4, End: 5}}, unordered: map[int]bool{}, skipped: map[string]int{}}
	fset := token.NewFileSet()
	mutations := []m.Mutation{{MutatedCode: []byte("x / 0")}, {MutatedCode: []byte("x / 1")}, {MutatedCode: []byte("x / 0.0")}}
	assert.Empty(t, c.keepBuilding(mgen.Arithmetic, []m.Mutation{{MutatedCode: []byte("x / 0")}}, nil, fset, content))

	kept := c.keepBuilding(mgen.Numbers, mutations, nil, fset, content)
	require.Len(t, kept, 1)
	assert.Equal(t, "x / 1", string(kept[0].MutatedCode))

	assert.Empty(t, c.keepBuilding(mgen.Numbers, nil, nil, fset, content))
	assert.Len(t, c.keepBuilding(mgen.Comparison, mutations[1:2], nil, fset, content), 1)

	c.logSkipped("p.go")

	assert.Contains(t, logs.String(), `msg="Skipped mutants that can't build" file=p.go mutagen=numbers count=2`)
	assert.Contains(t, logs.String(), "mutagen=arithmetic count=1")
	assert.NotContains(t, logs.String(), "mutagen=comparison", "a mutagen that skipped nothing")
}
