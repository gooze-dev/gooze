package domain_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
	m "gooze.dev/pkg/gooze/internal/model"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
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
	assert.Equal(t, 1, scope.ChangedTests(), "tested_test.go is the one changed test file")
	assert.Zero(t, (*domain.DiffScope)(nil).ChangedTests())
}

func TestDiffScope_NilIncludesEverything(t *testing.T) {
	t.Parallel()

	var scope *domain.DiffScope

	source := diffSource("/p/calc.go", "")
	assert.True(t, scope.InScope(source))
	assert.True(t, scope.Includes(source, 1))
}

func TestDiffScope_WithTests(t *testing.T) {
	t.Parallel()

	changes := domain.ChangeSet{Files: map[string]domain.FileChange{
		"/p/edited.go":           {Lines: []domain.LineRange{{Start: 3, End: 4}}},
		"/p/edited_test.go":      {Lines: []domain.LineRange{{Start: 10, End: 10}}},
		"/p/untouched_test.go":   {Lines: []domain.LineRange{{Start: 10, End: 10}}},
		"/p/onlydelete.go":       {},
		"/p/onlydelete_test.go":  {Lines: []domain.LineRange{{Start: 1, End: 1}}},
		"/p/created.go":          {Whole: true},
		"/p/created_test.go":     {Whole: true},
		"/p/other/unrelated.go":  {Lines: []domain.LineRange{{Start: 1, End: 1}}},
		"/p/other/unrelated.txt": {Whole: true},
	}}

	edited := diffSource("/p/edited.go", "/p/edited_test.go")
	untouched := diffSource("/p/untouched.go", "/p/untouched_test.go")
	onlyDelete := diffSource("/p/onlydelete.go", "/p/onlydelete_test.go")
	created := diffSource("/p/created.go", "/p/created_test.go")

	changed := &rpt.Scope{Lines: []rpt.LineRange{{Start: 3, End: 4}}}

	t.Run("file", func(t *testing.T) {
		t.Parallel()

		scope := domain.NewDiffScope(changes).WithTests(domain.DiffTestsFile)

		assert.True(t, scope.Includes(edited, 99), "a changed test widens its source to the whole file")
		assert.Nil(t, scope.ReportScope(edited))
		assert.True(t, scope.Includes(untouched, 99))
		assert.True(t, scope.InScope(untouched))
	})

	t.Run("unchanged", func(t *testing.T) {
		t.Parallel()

		scope := domain.NewDiffScope(changes).WithTests(domain.DiffTestsUnchanged)

		assert.True(t, scope.InScope(edited))
		assert.True(t, scope.Includes(edited, 3))
		assert.False(t, scope.Includes(edited, 99), "a source with changed lines of its own keeps to them")
		assert.Equal(t, changed, scope.ReportScope(edited))

		assert.True(t, scope.InScope(untouched), "a test-only change still widens its source")
		assert.True(t, scope.Includes(untouched, 99))
		assert.Nil(t, scope.ReportScope(untouched))

		assert.True(t, scope.InScope(onlyDelete), "a source that only lost lines has none of its own")
		assert.True(t, scope.Includes(onlyDelete, 99))

		assert.True(t, scope.Includes(created, 99), "a new file is in scope in full either way")
		assert.Nil(t, scope.ReportScope(created))
	})

	t.Run("off", func(t *testing.T) {
		t.Parallel()

		scope := domain.NewDiffScope(changes).WithTests(domain.DiffTestsOff)

		assert.True(t, scope.InScope(edited))
		assert.False(t, scope.Includes(edited, 99))
		assert.Equal(t, changed, scope.ReportScope(edited))

		assert.False(t, scope.InScope(untouched), "a changed test never puts its source in scope")
		assert.False(t, scope.Includes(untouched, 99))
		assert.False(t, scope.InScope(onlyDelete))

		assert.True(t, scope.Includes(created, 99))
	})

	t.Run("default", func(t *testing.T) {
		t.Parallel()

		assert.True(t, domain.NewDiffScope(changes).Includes(edited, 99), "without a policy a changed test widens its source")
		assert.True(t, domain.NewDiffScope(changes).WithTests("").Includes(edited, 99))
	})
}

func TestWorkflow_Estimate_DiffTestsPolicy(t *testing.T) {
	t.Parallel()

	calc := diffSource("/p/calc.go", "/p/calc_test.go")
	calc.Origin.ShortPath = "calc.go"
	calc.Origin.Hash = "h-calc"

	mutations := []m.Mutation{
		{ID: "c3", Source: calc, Type: mgen.Arithmetic, Line: 3},
		{ID: "c7", Source: calc, Type: mgen.Arithmetic, Line: 7},
	}

	for _, tt := range []struct {
		policy domain.DiffTests
		want   int
	}{
		{policy: domain.DiffTestsFile, want: 2},
		{policy: domain.DiffTestsUnchanged, want: 1},
		{policy: domain.DiffTestsOff, want: 1},
	} {
		t.Run(string(tt.policy), func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			fs := domainmocks.NewMockSourceFSAdapter(t)
			reporter := domainmocks.NewMockReporter(t)
			mutagen := domainmocks.NewMockMutagen(t)
			changes := domainmocks.NewMockChangeDetector(t)

			fs.EXPECT().Stream(mock.Anything, mock.Anything).Return(streamSources([]rpt.Source{calc}))
			mutagen.EXPECT().
				StreamMutations(mock.Anything, mock.Anything, mock.Anything, typeArgs(domain.DefaultMutations)...).
				RunAndReturn(streamMutationsFn(mutations))
			changes.EXPECT().Changes(mock.Anything, m.Path("."), "main").Return(domain.ChangeSet{
				Ref: "main", Base: "abc",
				Files: map[string]domain.FileChange{
					"/p/calc.go":      {Lines: []domain.LineRange{{Start: 3, End: 3}}},
					"/p/calc_test.go": {Lines: []domain.LineRange{{Start: 1, End: 1}}},
				},
			}, nil).Once()

			var got domain.Estimation

			reporter.EXPECT().StartEstimate(ctx).Return(nil).Once()
			reporter.EXPECT().DisplayEstimation(ctx, mock.Anything, nil).RunAndReturn(func(_ context.Context, e domain.Estimation, _ error) error {
				got = e
				return nil
			}).Once()
			reporter.EXPECT().Wait(ctx).Return().Once()
			reporter.EXPECT().Close(ctx).Return().Once()

			wf := domain.NewWorkflow(fs, domainmocks.NewMockReportStore(t), reporter, domainmocks.NewMockOrchestrator(t), mutagen, domain.WithChangeDetector(changes))

			require.NoError(t, wf.Estimate(ctx, domain.EstimateArgs{Paths: []m.Path{"./..."}, Diff: "main", DiffTests: tt.policy}))
			assert.Equal(t, tt.want, got.Total)
		})
	}
}
