package domain_test

import (
	"cmp"
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gooze.dev/pkg/gooze/internal/adapter"
	domain "gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
	m "gooze.dev/pkg/gooze/internal/model"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// cacheFixture is a reports directory holding results for a.go (two killed,
// one survived) and for b.go, a file outside the run's scope that still
// exists, so the cache keeps its reports.
type cacheFixture struct {
	reports string
	a, b    rpt.Source
}

func newCacheFixture(t *testing.T) cacheFixture {
	t.Helper()

	dir := t.TempDir()
	bPath := filepath.Join(dir, "b.go")
	require.NoError(t, os.WriteFile(bPath, []byte("package b\n"), 0o600))

	fx := cacheFixture{
		reports: filepath.Join(dir, "reports"),
		a:       rpt.Source{Origin: &rpt.File{ShortPath: "a.go", FullPath: "/p/a.go", Hash: "a1"}, Test: &rpt.File{Hash: "at"}},
		b:       rpt.Source{Origin: &rpt.File{ShortPath: "b.go", FullPath: bPath, Hash: "b1"}},
	}

	writeReports(t, fx.reports,
		m.Report{Source: fx.a, Result: m.Result{mgen.Arithmetic: {{MutationID: "a-1", Status: rpt.Killed}}}},
		m.Report{Source: fx.a, Result: m.Result{mgen.Arithmetic: {{MutationID: "a-2", Status: rpt.Killed}}}},
		m.Report{Source: fx.a, Result: m.Result{mgen.Arithmetic: {{MutationID: "a-3", Status: rpt.Survived}}}},
		m.Report{Source: fx.b, Result: m.Result{mgen.Arithmetic: {{MutationID: "b-1", Status: rpt.Survived}}}},
	)

	return fx
}

// cachedRunWorkflow wires a workflow over the real report store that scans
// sources and generates mutations per source key. It records the displayed
// score and estimation.
func cachedRunWorkflow(t *testing.T, sources []rpt.Source, mutations map[string][]m.Mutation, score *domain.MutationScore, estimation *domain.Estimation, missing ...mgen.Type) domain.Workflow {
	t.Helper()

	fs := domainmocks.NewMockSourceFSAdapter(t)
	fs.EXPECT().Stream(mock.Anything, mock.Anything).Return(streamSources(sources))

	stream := func(_ context.Context, source rpt.Source, _ domain.MutationOptions, fn func(m.Mutation) error, types ...mgen.Type) error {
		for _, mutation := range mutations[source.Key()] {
			// A mutation without a type is arithmetic, as in the fixture.
			if !slices.Contains(types, cmp.Or(mutation.Type, mgen.Arithmetic)) {
				continue
			}

			if err := fn(mutation); err != nil {
				return err
			}
		}

		return nil
	}

	// The run generates every default mutagen for the sources it tests, and
	// the cache asks for the ones a cached source has no results of: all
	// but arithmetic in the fixture, or missing when given.
	if missing == nil {
		missing = domain.DefaultMutations[1:]
	}

	mutagen := domainmocks.NewMockMutagen(t)
	mutagen.EXPECT().
		StreamMutations(mock.Anything, mock.Anything, mock.Anything, mock.Anything, typeArgs(domain.DefaultMutations)...).
		RunAndReturn(stream).Maybe()
	mutagen.EXPECT().
		StreamMutations(mock.Anything, mock.Anything, mock.Anything, mock.Anything, typeArgs(missing)...).
		RunAndReturn(stream).Maybe()

	workspace := domainmocks.NewMockWorkspace(t)
	workspace.EXPECT().Close(mock.Anything).Return().Maybe()
	workspace.EXPECT().Run(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, mutation m.Mutation) (m.Result, error) {
			return m.Result{mutation.Type: {{MutationID: mutation.ID, Status: rpt.Survived}}}, nil
		}).Maybe()

	orchestrator := domainmocks.NewMockOrchestrator(t)
	orchestrator.EXPECT().NewWorkspace(mock.Anything).Return(workspace).Maybe()

	reporter := domainmocks.NewMockReporter(t)
	reporter.EXPECT().StartTest(mock.Anything).Return(nil).Maybe()
	reporter.EXPECT().StartEstimate(mock.Anything).Return(nil).Maybe()
	reporter.EXPECT().Wait(mock.Anything).Return().Maybe()
	reporter.EXPECT().Close(mock.Anything).Return().Maybe()
	reporter.EXPECT().DisplayConcurrencyInfo(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return().Maybe()
	reporter.EXPECT().DisplayUpcomingTestsInfo(mock.Anything, mock.Anything).Return().Maybe()
	reporter.EXPECT().DisplayStartingTestInfo(mock.Anything, mock.Anything, mock.Anything).Return().Maybe()
	reporter.EXPECT().DisplayCompletedTestInfo(mock.Anything, mock.Anything, mock.Anything).Return().Maybe()
	reporter.EXPECT().DisplayMutationScore(mock.Anything, mock.Anything).
		Run(func(_ context.Context, s domain.MutationScore) { *score = s }).Maybe()
	reporter.EXPECT().DisplayEstimation(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, e domain.Estimation, _ error) error {
			*estimation = e
			return nil
		}).Maybe()

	return domain.NewWorkflow(fs, adapter.NewReportStore(), reporter, orchestrator, mutagen)
}

func cachedTestArgs(reports string) domain.TestArgs {
	return domain.TestArgs{
		EstimateArgs:    domain.EstimateArgs{Paths: []m.Path{"./..."}, UseCache: true, Reports: m.Path(reports)},
		Reports:         m.Path(reports),
		Threads:         1,
		TotalShardCount: 1,
	}
}

func TestWorkflow_Test_FullyCachedRunScoresStoredResultsInScope(t *testing.T) {
	fx := newCacheFixture(t)

	var (
		score      domain.MutationScore
		estimation domain.Estimation
	)

	wf := cachedRunWorkflow(t, []rpt.Source{fx.a}, nil, &score, &estimation)
	require.NoError(t, wf.Test(context.Background(), cachedTestArgs(fx.reports)))

	// b.go is outside the scanned scope, so its survivor does not count.
	assert.Equal(t, domain.MutationScore{Summary: rpt.Summary{Total: 3, Killed: 2, Survived: 1}, Cached: 3, Files: []domain.FileSummary{{Path: "a.go", Summary: rpt.Summary{Total: 3, Killed: 2, Survived: 1}}}}, score)
}

func TestWorkflow_Test_IncrementalRunScoresCachedAndTestedResults(t *testing.T) {
	fx := newCacheFixture(t)
	c := rpt.Source{Origin: &rpt.File{ShortPath: "c.go", FullPath: "/p/c.go", Hash: "c1"}}

	var (
		score      domain.MutationScore
		estimation domain.Estimation
	)

	mutations := map[string][]m.Mutation{"c.go": {{ID: "c-1", Source: c, Type: mgen.Arithmetic}}}
	wf := cachedRunWorkflow(t, []rpt.Source{fx.a, c}, mutations, &score, &estimation)
	require.NoError(t, wf.Test(context.Background(), cachedTestArgs(fx.reports)))

	assert.Equal(t, domain.MutationScore{Summary: rpt.Summary{Total: 4, Killed: 2, Survived: 2}, Cached: 3, Tested: 1, Files: []domain.FileSummary{{Path: "a.go", Summary: rpt.Summary{Total: 3, Killed: 2, Survived: 1}}, {Path: "c.go", Summary: rpt.Summary{Total: 1, Survived: 1}}}}, score)
}

func TestWorkflow_Test_RetestsACachedFileANewMutagenMutates(t *testing.T) {
	fx := newCacheFixture(t)

	var (
		score      domain.MutationScore
		estimation domain.Estimation
	)

	// a.go's reports hold arithmetic results only; a mutagen added since
	// mutates it too, so the whole file is tested again.
	mutations := map[string][]m.Mutation{"a.go": {
		{ID: "a-1", Source: fx.a, Type: mgen.Arithmetic},
		{ID: "a-4", Source: fx.a, Type: mgen.Bitwise},
	}}
	wf := cachedRunWorkflow(t, []rpt.Source{fx.a}, mutations, &score, &estimation)
	require.NoError(t, wf.Test(context.Background(), cachedTestArgs(fx.reports)))

	assert.Equal(t, domain.MutationScore{Summary: rpt.Summary{Total: 2, Survived: 2}, Tested: 2, Files: []domain.FileSummary{{Path: "a.go", Summary: rpt.Summary{Total: 2, Survived: 2}}}}, score)
}

// fullyTestedFixture is newCacheFixture with a.go's reports holding results
// of every default mutagen but skip.
func fullyTestedFixture(t *testing.T, skip mgen.Type) cacheFixture {
	t.Helper()

	fx := newCacheFixture(t)

	for _, mg := range domain.DefaultMutations {
		if mg != skip && mg != mgen.Arithmetic {
			writeReports(t, fx.reports, m.Report{Source: fx.a, Result: m.Result{mg: {{MutationID: "a-" + mg.Name, Status: rpt.Killed}}}})
		}
	}

	return fx
}

func TestWorkflow_Test_KeepsACachedFileWithResultsOfEveryMutagen(t *testing.T) {
	fx := fullyTestedFixture(t, mgen.Type{})

	var (
		score      domain.MutationScore
		estimation domain.Estimation
	)

	// The mutagen mock fails a call it doesn't expect, such as one asking
	// for no mutagens at all.
	wf := cachedRunWorkflow(t, []rpt.Source{fx.a}, nil, &score, &estimation)
	require.NoError(t, wf.Test(context.Background(), cachedTestArgs(fx.reports)))

	assert.Zero(t, score.Tested)
	assert.Equal(t, len(domain.DefaultMutations)+2, score.Cached)
}

func TestWorkflow_Test_RetestsACachedFileMissingOneMutagen(t *testing.T) {
	fx := fullyTestedFixture(t, mgen.Bitwise)

	var (
		score      domain.MutationScore
		estimation domain.Estimation
	)

	mutations := map[string][]m.Mutation{"a.go": {{ID: "a-new", Source: fx.a, Type: mgen.Bitwise}}}
	wf := cachedRunWorkflow(t, []rpt.Source{fx.a}, mutations, &score, &estimation, mgen.Bitwise)
	require.NoError(t, wf.Test(context.Background(), cachedTestArgs(fx.reports)))

	assert.Equal(t, 1, score.Tested)
	assert.Zero(t, score.Cached)
}

func TestWorkflow_Test_UncachedRunScoresTestedResultsOnly(t *testing.T) {
	fx := newCacheFixture(t)

	var (
		score      domain.MutationScore
		estimation domain.Estimation
	)

	mutations := map[string][]m.Mutation{"a.go": {{ID: "a-1", Source: fx.a, Type: mgen.Arithmetic}}}
	wf := cachedRunWorkflow(t, []rpt.Source{fx.a}, mutations, &score, &estimation)

	args := cachedTestArgs(fx.reports)
	args.UseCache = false
	require.NoError(t, wf.Test(context.Background(), args))

	assert.Equal(t, domain.MutationScore{Summary: rpt.Summary{Total: 1, Survived: 1}, Tested: 1, Files: []domain.FileSummary{{Path: "a.go", Summary: rpt.Summary{Total: 1, Survived: 1}}}}, score)
}

func TestWorkflow_Estimate_ListsCachedFiles(t *testing.T) {
	fx := newCacheFixture(t)
	c := rpt.Source{Origin: &rpt.File{ShortPath: "c.go", FullPath: "/p/c.go", Hash: "c1"}}

	var (
		score      domain.MutationScore
		estimation domain.Estimation
	)

	mutations := map[string][]m.Mutation{
		"a.go": {{ID: "a-1", Source: fx.a}, {ID: "a-2", Source: fx.a}},
		"c.go": {{ID: "c-1", Source: c}},
	}
	wf := cachedRunWorkflow(t, []rpt.Source{fx.a, c}, mutations, &score, &estimation)
	require.NoError(t, wf.Estimate(context.Background(), cachedTestArgs(fx.reports).EstimateArgs))

	assert.Equal(t, domain.Estimation{
		Total:  3,
		Cached: 2,
		Files: []domain.FileEstimate{
			{Path: "a.go", Count: 2, Cached: true},
			{Path: "c.go", Count: 1},
		},
	}, estimation)
}

func TestWorkflow_Test_UnsetShardCountScoresLikeAnUnshardedRun(t *testing.T) {
	fx := newCacheFixture(t)

	var (
		score      domain.MutationScore
		estimation domain.Estimation
	)

	wf := cachedRunWorkflow(t, []rpt.Source{fx.a}, nil, &score, &estimation)

	args := cachedTestArgs(fx.reports)
	args.TotalShardCount = 0
	require.NoError(t, wf.Test(context.Background(), args))

	assert.Equal(t, domain.MutationScore{Summary: rpt.Summary{Total: 3, Killed: 2, Survived: 1}, Cached: 3, Files: []domain.FileSummary{{Path: "a.go", Summary: rpt.Summary{Total: 3, Killed: 2, Survived: 1}}}}, score)
}

func TestWorkflow_Test_CachedRunWithEveryFileChangedHasNoCachedResults(t *testing.T) {
	fx := newCacheFixture(t)
	changedA := fx.a
	changedA.Origin = &rpt.File{ShortPath: "a.go", FullPath: "/p/a.go", Hash: "a2"}

	var (
		score      domain.MutationScore
		estimation domain.Estimation
	)

	mutations := map[string][]m.Mutation{"a.go": {{ID: "a-9", Source: changedA, Type: mgen.Arithmetic}}}
	wf := cachedRunWorkflow(t, []rpt.Source{changedA}, mutations, &score, &estimation)
	require.NoError(t, wf.Test(context.Background(), cachedTestArgs(fx.reports)))

	assert.Equal(t, domain.MutationScore{Summary: rpt.Summary{Total: 1, Survived: 1}, Tested: 1, Files: []domain.FileSummary{{Path: "a.go", Summary: rpt.Summary{Total: 1, Survived: 1}}}}, score)
}

func TestWorkflow_Estimate_WithoutCacheMarksNothingCached(t *testing.T) {
	fx := newCacheFixture(t)

	var (
		score      domain.MutationScore
		estimation domain.Estimation
	)

	mutations := map[string][]m.Mutation{"a.go": {{ID: "a-1", Source: fx.a}}}
	wf := cachedRunWorkflow(t, []rpt.Source{fx.a}, mutations, &score, &estimation)

	args := cachedTestArgs(fx.reports).EstimateArgs
	args.UseCache = false
	require.NoError(t, wf.Estimate(context.Background(), args))

	assert.Equal(t, domain.Estimation{Total: 1, Files: []domain.FileEstimate{{Path: "a.go", Count: 1}}}, estimation)
}

func TestWorkflow_Test_UnsetShardCountSavesReportsInTheBaseDir(t *testing.T) {
	fx := newCacheFixture(t)
	c := rpt.Source{Origin: &rpt.File{ShortPath: "c.go", FullPath: "/p/c.go", Hash: "c1"}}

	var (
		score      domain.MutationScore
		estimation domain.Estimation
	)

	mutations := map[string][]m.Mutation{"c.go": {{ID: "c-1", Source: c, Type: mgen.Arithmetic}}}
	wf := cachedRunWorkflow(t, []rpt.Source{fx.a, c}, mutations, &score, &estimation)

	args := cachedTestArgs(fx.reports)
	args.TotalShardCount = 0
	require.NoError(t, wf.Test(context.Background(), args))

	assert.Equal(t, domain.MutationScore{Summary: rpt.Summary{Total: 4, Killed: 2, Survived: 2}, Cached: 3, Tested: 1, Files: []domain.FileSummary{{Path: "a.go", Summary: rpt.Summary{Total: 3, Killed: 2, Survived: 1}}, {Path: "c.go", Summary: rpt.Summary{Total: 1, Survived: 1}}}}, score)
	assert.NoDirExists(t, filepath.Join(fx.reports, domain.ShardDirPrefix+"0"))
}
