package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"gooze.dev/pkg/gooze/internal/domain"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

func diffSource(origin, test string) rpt.Source {
	source := rpt.Source{Origin: &rpt.File{FullPath: origin}}
	if test != "" {
		source.Test = &rpt.File{FullPath: test}
	}

	return source
}

func TestDiffScope(t *testing.T) {
	t.Parallel()

	scope := domain.NewDiffScope(domain.ChangeSet{Ref: "main", Base: "abc", Files: map[string]domain.FileChange{
		"/p/calc.go":           {Lines: []domain.LineRange{{Start: 3, End: 4}}},
		"/p/new.go":            {Whole: true},
		"/p/tested_test.go":    {Lines: []domain.LineRange{{Start: 10, End: 10}}},
		"/p/notes.md":          {Whole: true},
		"/p/onlydelete.go":     {},
		"/p/other/unpaired.go": {Lines: []domain.LineRange{{Start: 1, End: 1}}},
	}})

	calc := diffSource("/p/calc.go", "/p/calc_test.go")
	assert.True(t, scope.InScope(calc))
	assert.False(t, scope.Includes(calc, 2))
	assert.True(t, scope.Includes(calc, 3))
	assert.True(t, scope.Includes(calc, 4))
	assert.False(t, scope.Includes(calc, 5))

	newFile := diffSource("/p/new.go", "")
	assert.True(t, scope.InScope(newFile))
	assert.True(t, scope.Includes(newFile, 99))

	// A changed test puts the whole source file it tests in scope.
	tested := diffSource("/p/tested.go", "/p/tested_test.go")
	assert.True(t, scope.InScope(tested))
	assert.True(t, scope.Includes(tested, 1))
	assert.True(t, scope.Includes(tested, 500))

	unchanged := diffSource("/p/same.go", "/p/same_test.go")
	assert.False(t, scope.InScope(unchanged))
	assert.False(t, scope.Includes(unchanged, 1))

	// A test whose change only deleted lines still puts its source in scope.
	deletedTest := domain.NewDiffScope(domain.ChangeSet{Files: map[string]domain.FileChange{"/p/x_test.go": {}}})
	assert.True(t, deletedTest.InScope(diffSource("/p/x.go", "/p/x_test.go")))
	assert.True(t, deletedTest.Includes(diffSource("/p/x.go", "/p/x_test.go"), 7))

	// A file whose change only deleted lines has nothing left to mutate.
	onlyDelete := diffSource("/p/onlydelete.go", "")
	assert.False(t, scope.InScope(onlyDelete))

	assert.False(t, scope.InScope(rpt.Source{}), "a source without an origin is never in scope")
	assert.False(t, scope.Includes(rpt.Source{}, 1))

	assert.Equal(t, domain.DiffInfo{Ref: "main", Base: "abc"}, scope.Info())
}

func TestDiffScope_NilIncludesEverything(t *testing.T) {
	t.Parallel()

	var scope *domain.DiffScope

	source := diffSource("/p/calc.go", "")
	assert.True(t, scope.InScope(source))
	assert.True(t, scope.Includes(source, 1))
}
