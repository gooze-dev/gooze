package domain_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	domain "gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
	m "gooze.dev/pkg/gooze/internal/model"
	pkg "gooze.dev/pkg/gooze/pkg"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// calcSource has mutations in InRange (lines 5-6), Parser.Next (line 12) and
// at package level (line 3).
var calcSource = rpt.Source{Origin: &rpt.File{FullPath: "/p/calc.go", ShortPath: "calc.go", Hash: "h-calc"}}

func calcMutations(types ...mgen.Type) []m.Mutation {
	all := []m.Mutation{
		{ID: "pkg", Source: calcSource, Type: mgen.Arithmetic, Line: 3},
		{ID: "in-a", Source: calcSource, Type: mgen.Arithmetic, Line: 5, Funcs: []string{"InRange"}},
		{ID: "in-c", Source: calcSource, Type: mgen.Comparison, Line: 6, Funcs: []string{"InRange"}},
		{ID: "next-c", Source: calcSource, Type: mgen.Comparison, Line: 12, Funcs: []string{"Next", "Parser.Next"}},
	}

	var kept []m.Mutation

	for _, mutation := range all {
		for _, t := range types {
			if mutation.Type == t {
				kept = append(kept, mutation)
			}
		}
	}

	return kept
}

// expectStream makes the mutagen stream calcMutations for exactly types.
func expectStream(mutagen *domainmocks.MockMutagen, types ...mgen.Type) {
	args := []any{mock.Anything, mock.Anything, mock.Anything}
	for _, t := range types {
		args = append(args, t)
	}

	mutagen.On("StreamMutations", args...).Return(func(ctx context.Context, source rpt.Source, fn func(m.Mutation) error, ts ...mgen.Type) error {
		return streamMutationsFn(calcMutations(ts...))(ctx, source, fn, ts...)
	})
}

func TestWorkflow_Estimate_RunPatternCountsMatchingFunctions(t *testing.T) {
	ctx := context.Background()
	fs := domainmocks.NewMockSourceFSAdapter(t)
	reporter := domainmocks.NewMockReporter(t)
	mutagen := domainmocks.NewMockMutagen(t)

	fs.EXPECT().Stream(mock.Anything, mock.Anything).Return(streamSources([]rpt.Source{calcSource}))
	expectStream(mutagen, domain.DefaultMutations...)

	var got domain.Estimation

	reporter.EXPECT().StartEstimate(ctx).Return(nil)
	reporter.EXPECT().DisplayEstimation(ctx, mock.Anything, nil).RunAndReturn(func(_ context.Context, e domain.Estimation, _ error) error {
		got = e
		return nil
	})
	reporter.EXPECT().Wait(ctx).Return()
	reporter.EXPECT().Close(ctx).Return()

	wf := domain.NewWorkflow(fs, domainmocks.NewMockReportStore(t), reporter, domainmocks.NewMockOrchestrator(t), mutagen)
	require.NoError(t, wf.Estimate(ctx, domain.EstimateArgs{Run: "^InRange$"}))

	assert.Equal(t, 2, got.Total)
	assert.Equal(t, "^InRange$", got.Run)
	assert.Nil(t, got.Mutagens)
}

func TestWorkflow_Test_RunAndMutagensSelectAndRecordScope(t *testing.T) {
	ctx := context.Background()
	fs := domainmocks.NewMockSourceFSAdapter(t)
	store := domainmocks.NewMockReportStore(t)
	reporter := domainmocks.NewMockReporter(t)
	mutagen := domainmocks.NewMockMutagen(t)
	orchestrator := domainmocks.NewMockOrchestrator(t)
	workspace := domainmocks.NewMockWorkspace(t)

	fs.EXPECT().Stream(mock.Anything, mock.Anything).Return(streamSources([]rpt.Source{calcSource}))
	// Only the comparison mutagen is asked for: the others are never generated.
	expectStream(mutagen, mgen.Comparison)

	for _, call := range []string{"StartTest", "Wait", "Close", "DisplayConcurrencyInfo", "DisplayUpcomingTestsInfo", "DisplayStartingTestInfo", "DisplayCompletedTestInfo", "DisplayMutationScore"} {
		reporter.On(call, mock.Anything).Return(nil).Maybe()
		reporter.On(call, mock.Anything, mock.Anything).Return().Maybe()
		reporter.On(call, mock.Anything, mock.Anything, mock.Anything).Return().Maybe()
		reporter.On(call, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return().Maybe()
	}

	var tested []string

	workspace.EXPECT().Run(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, mutation m.Mutation) (m.Result, error) {
		tested = append(tested, mutation.ID)
		return m.Result{mutation.Type: {{MutationID: mutation.ID, Status: rpt.Killed}}}, nil
	})
	workspace.EXPECT().Close(mock.Anything).Return().Maybe()
	orchestrator.EXPECT().NewWorkspace().Return(workspace).Maybe()

	var saved []m.Report

	store.EXPECT().CleanReports(mock.Anything, mock.Anything, mock.Anything).Return(nil)
	store.EXPECT().SaveReports(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, _ m.Path, reports pkg.FileSpill[m.Report]) error {
		return reports.Range(func(_ uint64, report m.Report) error {
			saved = append(saved, report)
			return nil
		})
	})
	store.EXPECT().RegenerateIndex(mock.Anything, mock.Anything).Return(nil)

	wf := domain.NewWorkflow(fs, store, reporter, orchestrator, mutagen)
	require.NoError(t, wf.Test(ctx, domain.TestArgs{
		EstimateArgs: domain.EstimateArgs{Run: "Next$", Mutagens: []string{"comparison"}},
		Reports:      ".gooze-reports", Threads: 1, TotalShardCount: 1,
	}))

	assert.Equal(t, []string{"next-c"}, tested)
	require.Len(t, saved, 1)
	assert.Equal(t, &rpt.Scope{Run: "Next$", Mutagens: []string{"comparison"}}, saved[0].Scope,
		"the report is marked partial, so a full run re-tests the file")
}

func TestWorkflow_SelectionErrorsStopBeforeScanning(t *testing.T) {
	ctx := context.Background()

	cases := map[string]struct {
		args domain.EstimateArgs
		want string
	}{
		"bad pattern":     {domain.EstimateArgs{Run: "(unclosed"}, `invalid --run pattern "(unclosed"`},
		"unknown mutagen": {domain.EstimateArgs{Mutagens: []string{"bogus"}}, `unknown mutagen "bogus"`},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			reporter := domainmocks.NewMockReporter(t)
			reporter.EXPECT().StartEstimate(ctx).Return(nil)
			reporter.EXPECT().Close(ctx).Return()

			wf := domain.NewWorkflow(domainmocks.NewMockSourceFSAdapter(t), domainmocks.NewMockReportStore(t), reporter,
				domainmocks.NewMockOrchestrator(t), domainmocks.NewMockMutagen(t))

			err := wf.Estimate(ctx, tc.args)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			assert.Equal(t, m.ExitUsage, exitCodeOf(err))
		})
	}
}

// narrowedRunFixture runs one narrowed test over calcSource with a cached
// store: CheckUpdates reports calc.go changed and gone.go deleted.
type narrowedRunFixture struct {
	wf      domain.Workflow
	base    string
	saved   map[string]int // reports saved, per directory
	cleaned []string       // directories CleanReports touched
	indexed []string       // directories whose index was regenerated
	score   domain.MutationScore
}

func newNarrowedRunFixture(t *testing.T, types ...mgen.Type) *narrowedRunFixture {
	t.Helper()

	f := &narrowedRunFixture{base: t.TempDir(), saved: map[string]int{}}
	fs := domainmocks.NewMockSourceFSAdapter(t)
	store := domainmocks.NewMockReportStore(t)
	reporter := domainmocks.NewMockReporter(t)
	mutagen := domainmocks.NewMockMutagen(t)
	orchestrator := domainmocks.NewMockOrchestrator(t)
	workspace := domainmocks.NewMockWorkspace(t)

	fs.EXPECT().Stream(mock.Anything, mock.Anything).Return(streamSources([]rpt.Source{calcSource}))
	expectStream(mutagen, types...)

	gone := rpt.Source{Origin: &rpt.File{FullPath: "/p/gone.go", ShortPath: "gone.go", Hash: "h-gone"}}
	store.EXPECT().CheckUpdates(mock.Anything, m.Path(f.base), mock.Anything).RunAndReturn(
		func(context.Context, m.Path, pkg.FileSpill[rpt.Source]) (pkg.FileSpill[rpt.Source], pkg.FileSpill[rpt.Source], error) {
			return spillOfSources(t, calcSource), spillOfSources(t, gone), nil
		})
	store.EXPECT().CleanReports(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, path m.Path, _ pkg.FileSpill[rpt.Source]) error {
		f.cleaned = append(f.cleaned, string(path))
		return nil
	}).Maybe()
	store.EXPECT().SaveReports(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, path m.Path, reports pkg.FileSpill[m.Report]) error {
		f.saved[string(path)] += int(reports.Len()) //nolint:gosec // test sizes are tiny
		return nil
	})
	store.EXPECT().RegenerateIndex(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, path m.Path) error {
		f.indexed = append(f.indexed, string(path))
		return nil
	})

	for _, call := range []string{"StartTest", "Wait", "Close", "DisplayConcurrencyInfo", "DisplayUpcomingTestsInfo", "DisplayStartingTestInfo", "DisplayCompletedTestInfo"} {
		reporter.On(call, mock.Anything).Return(nil).Maybe()
		reporter.On(call, mock.Anything, mock.Anything).Return().Maybe()
		reporter.On(call, mock.Anything, mock.Anything, mock.Anything).Return().Maybe()
		reporter.On(call, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return().Maybe()
	}

	reporter.EXPECT().DisplayMutationScore(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, score domain.MutationScore) {
		f.score = score
	})

	workspace.EXPECT().Run(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, mutation m.Mutation) (m.Result, error) {
		return m.Result{mutation.Type: {{MutationID: mutation.ID, Status: rpt.Survived}}}, nil
	})
	workspace.EXPECT().Close(mock.Anything).Return().Maybe()
	orchestrator.EXPECT().NewWorkspace().Return(workspace).Maybe()

	f.wf = domain.NewWorkflow(fs, store, reporter, orchestrator, mutagen)

	return f
}

func spillOfSources(t *testing.T, sources ...rpt.Source) pkg.FileSpill[rpt.Source] {
	t.Helper()

	spill, err := pkg.NewFileSpill[rpt.Source]()
	require.NoError(t, err)

	for _, source := range sources {
		require.NoError(t, spill.Append(source))
	}

	return spill
}

func TestWorkflow_Test_NarrowedRunWritesItsOwnReportsDir(t *testing.T) {
	f := newNarrowedRunFixture(t, domain.DefaultMutations...)
	runDir := filepath.Join(f.base, domain.RunDirName)

	// A previous narrowed run left reports behind: the new run replaces them.
	require.NoError(t, os.MkdirAll(runDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(runDir, "old.yaml"), []byte("source: {}\n"), 0o600))

	require.NoError(t, f.wf.Test(context.Background(), domain.TestArgs{
		EstimateArgs: domain.EstimateArgs{Run: "^InRange$", UseCache: true, Reports: m.Path(f.base)},
		Reports:      m.Path(f.base), Threads: 1, TotalShardCount: 1,
	}))

	assert.Equal(t, map[string]int{runDir: 2}, f.saved, "only InRange's two mutations, saved under run/")
	assert.Equal(t, []string{runDir}, f.indexed)
	assert.NotContains(t, f.cleaned, f.base, "a narrowed run never changes the main reports, not even for deleted files")
	assert.NoFileExists(t, filepath.Join(runDir, "old.yaml"))

	// The score covers only this run's selection, not the stored reports.
	assert.Equal(t, 2, f.score.Total)
	assert.Equal(t, 2, f.score.Tested)
	assert.Equal(t, "functions matching ^InRange$", f.score.Selection)
	assert.Equal(t, runDir, string(f.score.ReportsDir))
}

func TestWorkflow_Test_NarrowedShardWritesUnderRunDir(t *testing.T) {
	f := newNarrowedRunFixture(t, mgen.Comparison)

	require.NoError(t, f.wf.Test(context.Background(), domain.TestArgs{
		EstimateArgs: domain.EstimateArgs{Mutagens: []string{"comparison"}, UseCache: true, Reports: m.Path(f.base)},
		Reports:      m.Path(f.base), Threads: 1, ShardIndex: 1, TotalShardCount: 2,
	}))

	want := filepath.Join(f.base, domain.RunDirName, domain.ShardDirPrefix+"1")
	for dir := range f.saved {
		assert.Equal(t, want, dir)
	}

	assert.Equal(t, []string{want}, f.indexed)
}
