package domain_test

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
	"gooze.dev/pkg/gooze/pkg/config"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

func TestSelection_Run(t *testing.T) {
	t.Parallel()

	sel := domain.Selection{Run: regexp.MustCompile(`^InRange$|^Parser\.Next$`)}
	src := rpt.Source{Origin: &rpt.File{FullPath: "/p/calc.go"}}

	assert.True(t, sel.Includes(m.Mutation{Source: src, Funcs: []string{"InRange"}}))
	assert.True(t, sel.Includes(m.Mutation{Source: src, Funcs: []string{"Next", "Parser.Next"}}), "methods match by Type.Name")
	assert.False(t, sel.Includes(m.Mutation{Source: src, Funcs: []string{"Next", "Lexer.Next"}}))
	assert.False(t, sel.Includes(m.Mutation{Source: src, Funcs: []string{"InRangeAll"}}), "the pattern is the user's regex: anchors decide")
	assert.False(t, sel.Includes(m.Mutation{Source: src}), "package-level code is in no function")

	// Like go test -run, the pattern is unanchored.
	loose := domain.Selection{Run: regexp.MustCompile(`Range`)}
	assert.True(t, loose.Includes(m.Mutation{Source: src, Funcs: []string{"InRangeAll"}}))
}

func TestSelection_NoFiltersIncludeEverything(t *testing.T) {
	t.Parallel()

	var sel domain.Selection

	src := rpt.Source{Origin: &rpt.File{FullPath: "/p/calc.go"}}
	assert.True(t, sel.InScope(src))
	assert.True(t, sel.Includes(m.Mutation{Source: src}))
	assert.Nil(t, sel.ReportScope(src))
	assert.Equal(t, domain.DefaultMutations, sel.Types())
}

func TestSelection_Types(t *testing.T) {
	t.Parallel()

	sel := domain.Selection{Mutagens: []mgen.Type{mgen.Comparison, mgen.Arithmetic}}
	assert.Equal(t, []mgen.Type{mgen.Comparison, mgen.Arithmetic}, sel.Types())
}

func TestParseMutagens(t *testing.T) {
	t.Parallel()

	types, err := domain.ParseMutagens([]string{"comparison, Arithmetic", "comparison", ""})
	require.NoError(t, err)
	assert.Equal(t, []mgen.Type{mgen.Comparison, mgen.Arithmetic}, types, "comma-separated, case-insensitive, de-duplicated, in the order given")

	none, err := domain.ParseMutagens(nil)
	require.NoError(t, err)
	assert.Nil(t, none, "no names means every default mutagen")

	_, err = domain.ParseMutagens([]string{"comparison", "bogus"})
	require.EqualError(t, err, `unknown mutagen "bogus"; gooze runs arithmetic, boolean, numbers, comparison, logical, unary`)

	_, err = domain.ParseMutagens([]string{"statement"})
	require.EqualError(t, err, `unknown mutagen "statement"; gooze runs arithmetic, boolean, numbers, comparison, logical, unary`,
		"registered but not run by gooze yet")
}

func TestSelection_ReportScope(t *testing.T) {
	t.Parallel()

	calc := rpt.Source{Origin: &rpt.File{FullPath: "/p/calc.go"}}
	newFile := rpt.Source{Origin: &rpt.File{FullPath: "/p/new.go"}}
	diff := domain.NewDiffScope(domain.ChangeSet{Ref: "main", Base: "abc", Files: map[string]domain.FileChange{
		"/p/calc.go": {Lines: []domain.LineRange{{Start: 3, End: 4}}},
		"/p/new.go":  {Whole: true},
	}})
	run := regexp.MustCompile(`^InRange$`)
	mutagens := []mgen.Type{mgen.Comparison}

	cases := map[string]struct {
		sel    domain.Selection
		source rpt.Source
		want   *rpt.Scope
	}{
		"run only":       {domain.Selection{Run: run}, calc, &rpt.Scope{Run: "^InRange$"}},
		"mutagens only":  {domain.Selection{Mutagens: mutagens}, calc, &rpt.Scope{Mutagens: []string{"comparison"}}},
		"diff lines":     {domain.Selection{Diff: diff}, calc, &rpt.Scope{Base: "abc", Lines: []rpt.LineRange{{Start: 3, End: 4}}}},
		"diff whole":     {domain.Selection{Diff: diff}, newFile, nil},
		"diff whole+run": {domain.Selection{Diff: diff, Run: run}, newFile, &rpt.Scope{Run: "^InRange$"}},
		"all three": {domain.Selection{Diff: diff, Run: run, Mutagens: mutagens}, calc,
			&rpt.Scope{Base: "abc", Lines: []rpt.LineRange{{Start: 3, End: 4}}, Run: "^InRange$", Mutagens: []string{"comparison"}}},
	}

	for name, tc := range cases {
		assert.Equal(t, tc.want, tc.sel.ReportScope(tc.source), name)
	}
}

func TestSelection_NarrowedAndDescribe(t *testing.T) {
	t.Parallel()

	assert.False(t, domain.Selection{}.Narrowed())
	assert.Empty(t, domain.Selection{}.Describe())

	diff := domain.NewDiffScope(domain.ChangeSet{Ref: "origin/main", Base: "abc"})
	sel := domain.Selection{Diff: diff, Run: regexp.MustCompile(`^Add$`), Mutagens: []mgen.Type{mgen.Comparison, mgen.Logical}}

	assert.True(t, sel.Narrowed())
	assert.Equal(t, "changed lines since origin/main; functions matching ^Add$; mutagens comparison, logical", sel.Describe())

	assert.True(t, domain.Selection{Run: regexp.MustCompile(`x`)}.Narrowed())
	assert.True(t, domain.Selection{Mutagens: []mgen.Type{mgen.Unary}}.Narrowed())
	assert.True(t, domain.Selection{Diff: diff}.Narrowed())
}

func TestParseMutagensMatchesTheConfigSchema(t *testing.T) {
	t.Parallel()

	field, ok := config.Lookup(config.KeyRunMutagens)
	require.True(t, ok)

	// The schema lists the names editors validate run.mutagens against; they
	// must be exactly the mutagens a run accepts.
	types, err := domain.ParseMutagens(field.Enum)
	require.NoError(t, err)
	assert.Equal(t, domain.DefaultMutations, types)
}
