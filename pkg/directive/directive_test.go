package directive

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func names(d Directive) []string {
	out := make([]string, 0, len(d.Names))
	for _, n := range d.Names {
		out = append(out, n.Text)
	}

	return out
}

func TestParse(t *testing.T) {
	cases := []struct {
		text  string
		ok    bool
		names []string
	}{
		{text: "//gooze:ignore", ok: true, names: []string{}},
		{text: "// gooze:ignore", ok: true, names: []string{}},
		{text: "// gooze:ignore a, b", ok: true, names: []string{"a", "b"}},
		{text: "//gooze:ignore arithmetic comparison", ok: true, names: []string{"arithmetic", "comparison"}},
		{text: "//gooze:ignore Arithmetic, COMPARISON ", ok: true, names: []string{"Arithmetic", "COMPARISON"}},
		{text: "/* gooze:ignore x */", ok: true, names: []string{"x"}},
		{text: "/*gooze:ignore*/", ok: true, names: []string{}},
		{text: "//gooze:ignore\tnumbers", ok: true, names: []string{"numbers"}},
		{text: "//gooze:ignore a+b, c#d", ok: true, names: []string{"a+b", "c#d"}},
		{text: "//gooze:ignore , ,", ok: true, names: []string{}},
		{text: "  //  gooze:ignore a  ", ok: true, names: []string{"a"}},
		{text: "gooze:ignore a", ok: true, names: []string{"a"}},
		{text: "//gooze:ignorearithmetic", ok: false},
		{text: "//gooze:ignore,arithmetic", ok: false},
		{text: "//gooze:Ignore", ok: false},
		{text: "// note gooze:ignore", ok: false},
		{text: "/// gooze:ignore", ok: false},
		{text: "// gooze: ignore", ok: false},
		{text: "", ok: false},
	}

	for _, tc := range cases {
		d, ok := Parse(tc.text)
		require.Equal(t, tc.ok, ok, tc.text)

		if tc.ok {
			assert.Equal(t, tc.names, names(d), tc.text)
			assert.Equal(t, len(tc.names) == 0, d.IgnoresAll(), tc.text)
		}
	}
}

func TestParse_Offsets(t *testing.T) {
	text := "// gooze:ignore Arithmetic,  comparison"

	d, ok := Parse(text)
	require.True(t, ok)
	assert.Equal(t, 3, d.Keyword)
	assert.Equal(t, len(text), d.End)
	assert.Equal(t, []Name{
		{Text: "Arithmetic", Start: 16, End: 26},
		{Text: "comparison", Start: 29, End: 39},
	}, d.Names)

	for _, n := range d.Names {
		assert.Equal(t, n.Text, text[n.Start:n.End])
	}

	assert.Equal(t, Keyword, text[d.Keyword:d.Keyword+len(Keyword)])
}

func TestParse_BlockCommentEndExcludesCloser(t *testing.T) {
	text := "/* gooze:ignore x  */"

	d, ok := Parse(text)
	require.True(t, ok)
	assert.Equal(t, 3, d.Keyword)
	assert.Equal(t, 17, d.End)
	assert.Equal(t, []Name{{Text: "x", Start: 16, End: 17}}, d.Names)
}

func TestParse_EmptyListEndsAfterKeyword(t *testing.T) {
	d, ok := Parse("//gooze:ignore   ")
	require.True(t, ok)
	assert.Equal(t, 2, d.Keyword)
	assert.Equal(t, 14, d.End)
	assert.Empty(t, d.Names)
}

func TestDirective_Ignores(t *testing.T) {
	d, ok := Parse("//gooze:ignore Arithmetic")
	require.True(t, ok)
	assert.True(t, d.Ignores("arithmetic"))
	assert.True(t, d.Ignores("ARITHMETIC"))
	assert.False(t, d.Ignores("boolean"))

	all, ok := Parse("//gooze:ignore")
	require.True(t, ok)
	assert.True(t, all.Ignores("boolean"))
}

func TestParseLine(t *testing.T) {
	line := "\t_ = a + 1 // gooze:ignore arithmetic"

	d, ok := ParseLine(line)
	require.True(t, ok)
	assert.Equal(t, Keyword, line[d.Keyword:d.Keyword+len(Keyword)])
	assert.Equal(t, len(line), d.End)
	require.Len(t, d.Names, 1)
	assert.Equal(t, "arithmetic", line[d.Names[0].Start:d.Names[0].End])
}

func TestParseLine_BlockCommentBeforeCode(t *testing.T) {
	line := "x := /* gooze:ignore numbers */ 5"

	d, ok := ParseLine(line)
	require.True(t, ok)
	assert.Equal(t, 8, d.Keyword)
	assert.Equal(t, 28, d.End)
	assert.Equal(t, []Name{{Text: "numbers", Start: 21, End: 28}}, d.Names)
}

func TestParseLine_NotDirectives(t *testing.T) {
	for _, line := range []string{
		"",
		"x := 1",
		`s := "// gooze:ignore"`,
		"// note // gooze:ignore",
		"//gooze:ignorearithmetic",
		"/* a */ x := 1 // b",
	} {
		_, ok := ParseLine(line)
		assert.False(t, ok, line)
	}
}

func TestParseLine_SecondComment(t *testing.T) {
	line := "/* a */ x := 1 //gooze:ignore"

	d, ok := ParseLine(line)
	require.True(t, ok)
	assert.Equal(t, 17, d.Keyword)
	assert.True(t, d.IgnoresAll())
}

func TestParseLine_SkipsIllegalTokensBeforeComment(t *testing.T) {
	d, ok := ParseLine("x := 1 @ //gooze:ignore arithmetic")
	require.True(t, ok)
	require.Len(t, d.Names, 1)
	assert.Equal(t, "arithmetic", d.Names[0].Text)
}

func TestDirective_IgnoresAllWithEmptyOrNilNames(t *testing.T) {
	assert.True(t, Directive{}.IgnoresAll())
	assert.True(t, Directive{Names: []Name{}}.IgnoresAll())
	assert.False(t, Directive{Names: []Name{{Text: "a"}}}.IgnoresAll())
}
