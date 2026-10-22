package domain_test

import (
	"context"
	"sync"
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

// diffEstimateFixture wires a workflow whose scan yields calc.go (lines 3 and 7
// mutated) and same.go (line 2), with a change detector that reports line 3
// of calc.go as changed.
type diffEstimateFixture struct {
	reporter *domainmocks.MockReporter
	changes  *domainmocks.MockChangeDetector
	wf       domain.Workflow
}

func newDiffEstimateFixture(t *testing.T) diffEstimateFixture {
	t.Helper()

	fs := domainmocks.NewMockSourceFSAdapter(t)
	store := domainmocks.NewMockReportStore(t)
	reporter := domainmocks.NewMockReporter(t)
	mutagen := domainmocks.NewMockMutagen(t)
	changes := domainmocks.NewMockChangeDetector(t)

	calc := rpt.Source{Origin: &rpt.File{FullPath: "/p/calc.go", ShortPath: "calc.go", Hash: "h-calc"}}
	same := rpt.Source{Origin: &rpt.File{FullPath: "/p/same.go", ShortPath: "same.go", Hash: "h-same"}}

	fs.EXPECT().Stream(mock.Anything, mock.Anything).Return(streamSources([]rpt.Source{calc, same})).Maybe()

	byPath := map[string][]m.Mutation{
		"/p/calc.go": {
			{ID: "c3a", Source: calc, Type: mgen.Arithmetic, Line: 3},
			{ID: "c3b", Source: calc, Type: mgen.Comparison, Line: 3},
			{ID: "c7", Source: calc, Type: mgen.Arithmetic, Line: 7},
		},
		"/p/same.go": {{ID: "s2", Source: same, Type: mgen.Arithmetic, Line: 2}},
	}

	mutagen.EXPECT().
		StreamMutations(mock.Anything, mock.Anything, mock.Anything, typeArgs(domain.DefaultMutations)...).
		RunAndReturn(func(ctx context.Context, source rpt.Source, fn func(m.Mutation) error, types ...mgen.Type) error {
			return streamMutationsFn(byPath[source.Origin.FullPath])(ctx, source, fn, types...)
		}).Maybe()

	wf := domain.NewWorkflow(fs, store, reporter, domainmocks.NewMockOrchestrator(t), mutagen, domain.WithChangeDetector(changes))

	return diffEstimateFixture{reporter: reporter, changes: changes, wf: wf}
}

func TestWorkflow_Estimate_DiffCountsOnlyChangedLines(t *testing.T) {
	f := newDiffEstimateFixture(t)
	ctx := context.Background()

	f.changes.EXPECT().Changes(mock.Anything, m.Path("."), "origin/main").Return(domain.ChangeSet{
		Ref: "origin/main", Base: "1a2b3c4d5e",
		Files: map[string]domain.FileChange{"/p/calc.go": {Lines: []domain.LineRange{{Start: 3, End: 3}}}},
	}, nil).Once()

	var got domain.Estimation

	f.reporter.EXPECT().StartEstimate(ctx).Return(nil).Once()
	f.reporter.EXPECT().DisplayEstimation(ctx, mock.Anything, nil).RunAndReturn(func(_ context.Context, e domain.Estimation, _ error) error {
		got = e
		return nil
	}).Once()
	f.reporter.EXPECT().Wait(ctx).Return().Once()
	f.reporter.EXPECT().Close(ctx).Return().Once()

	require.NoError(t, f.wf.Estimate(ctx, domain.EstimateArgs{Paths: []m.Path{"./..."}, Diff: "origin/main"}))

	assert.Equal(t, 2, got.Total, "only the two mutations on calc.go line 3")
	assert.Equal(t, []domain.FileEstimate{{Path: "calc.go", Count: 2}}, got.Files)
	assert.Equal(t, &domain.DiffInfo{Ref: "origin/main", Base: "1a2b3c4d5e", Files: 1}, got.Diff)
}

func TestWorkflow_Estimate_DiffWithNoChanges(t *testing.T) {
	f := newDiffEstimateFixture(t)
	ctx := context.Background()

	f.changes.EXPECT().Changes(mock.Anything, mock.Anything, "HEAD").Return(domain.ChangeSet{Ref: "HEAD", Base: "abc"}, nil).Once()

	f.reporter.EXPECT().StartEstimate(ctx).Return(nil).Once()
	f.reporter.EXPECT().DisplayEstimation(ctx, mock.MatchedBy(func(e domain.Estimation) bool {
		return e.Total == 0 && len(e.Files) == 0 && e.Diff != nil && e.Diff.Files == 0
	}), nil).Return(nil).Once()
	f.reporter.EXPECT().Wait(ctx).Return().Once()
	f.reporter.EXPECT().Close(ctx).Return().Once()

	require.NoError(t, f.wf.Estimate(ctx, domain.EstimateArgs{Diff: "HEAD"}))
}

func TestWorkflow_Estimate_WithoutDiffIgnoresDetector(t *testing.T) {
	f := newDiffEstimateFixture(t)
	ctx := context.Background()

	f.reporter.EXPECT().StartEstimate(ctx).Return(nil).Once()
	f.reporter.EXPECT().DisplayEstimation(ctx, mock.MatchedBy(func(e domain.Estimation) bool {
		return e.Total == 4 && e.Diff == nil
	}), nil).Return(nil).Once()
	f.reporter.EXPECT().Wait(ctx).Return().Once()
	f.reporter.EXPECT().Close(ctx).Return().Once()

	require.NoError(t, f.wf.Estimate(ctx, domain.EstimateArgs{}))
	f.changes.AssertNotCalled(t, "Changes", mock.Anything, mock.Anything, mock.Anything)
}

func TestWorkflow_Estimate_DiffErrorStopsBeforeScanning(t *testing.T) {
	f := newDiffEstimateFixture(t)
	ctx := context.Background()

	f.changes.EXPECT().Changes(mock.Anything, mock.Anything, "main").Return(domain.ChangeSet{}, domain.ErrGitNotInstalled).Once()
	f.reporter.EXPECT().StartEstimate(ctx).Return(nil).Once()
	f.reporter.EXPECT().Close(ctx).Return().Once()

	err := f.wf.Estimate(ctx, domain.EstimateArgs{Diff: "main"})
	require.ErrorIs(t, err, domain.ErrGitNotInstalled)
}

func TestWorkflow_Estimate_DiffWithoutDetector(t *testing.T) {
	ctx := context.Background()
	reporter := domainmocks.NewMockReporter(t)
	reporter.EXPECT().StartEstimate(ctx).Return(nil).Once()
	reporter.EXPECT().Close(ctx).Return().Once()

	wf := domain.NewWorkflow(domainmocks.NewMockSourceFSAdapter(t), domainmocks.NewMockReportStore(t), reporter,
		domainmocks.NewMockOrchestrator(t), domainmocks.NewMockMutagen(t))

	err := wf.Estimate(ctx, domain.EstimateArgs{Diff: "main"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "diff mode")
}

// diffRunFixture wires a test run over three sources: calc.go (mutations on
// lines 3 and 7, line 3 changed), same.go (unchanged) and tested.go (its test
// file changed, so in scope in full).
type diffRunFixture struct {
	wf      domain.Workflow
	changes *domainmocks.MockChangeDetector
	store   *domainmocks.MockReportStore
	fs      *domainmocks.MockSourceFSAdapter

	mu        sync.Mutex
	tested    []string
	cleaned   []string
	saved     []m.Report
	selection *rpt.Selection
}

var diffChanges = domain.ChangeSet{
	Ref: "origin/main", Base: "1a2b3c4d",
	Files: map[string]domain.FileChange{
		"/p/calc.go":        {Lines: []domain.LineRange{{Start: 3, End: 3}}},
		"/p/tested_test.go": {Lines: []domain.LineRange{{Start: 9, End: 9}}},
	},
}

func newDiffRunFixture(t *testing.T) *diffRunFixture {
	t.Helper()

	f := &diffRunFixture{
		changes: domainmocks.NewMockChangeDetector(t),
		store:   domainmocks.NewMockReportStore(t),
		fs:      domainmocks.NewMockSourceFSAdapter(t),
	}

	reporter := domainmocks.NewMockReporter(t)
	mutagen := domainmocks.NewMockMutagen(t)
	orchestrator := domainmocks.NewMockOrchestrator(t)
	workspace := domainmocks.NewMockWorkspace(t)

	calc := rpt.Source{Origin: &rpt.File{FullPath: "/p/calc.go", ShortPath: "calc.go", Hash: "h-calc"}}
	same := rpt.Source{Origin: &rpt.File{FullPath: "/p/same.go", ShortPath: "same.go", Hash: "h-same"}}
	tested := rpt.Source{
		Origin: &rpt.File{FullPath: "/p/tested.go", ShortPath: "tested.go", Hash: "h-tested"},
		Test:   &rpt.File{FullPath: "/p/tested_test.go", ShortPath: "tested_test.go", Hash: "h-tested-test"},
	}

	f.fs.EXPECT().Stream(mock.Anything, mock.Anything).Return(streamSources([]rpt.Source{calc, same, tested})).Maybe()

	byPath := map[string][]m.Mutation{
		"/p/calc.go": {
			{ID: "c3a", Source: calc, Type: mgen.Arithmetic, Line: 3},
			{ID: "c3b", Source: calc, Type: mgen.Comparison, Line: 3},
			{ID: "c7", Source: calc, Type: mgen.Arithmetic, Line: 7},
		},
		"/p/same.go":   {{ID: "s2", Source: same, Type: mgen.Arithmetic, Line: 2}},
		"/p/tested.go": {{ID: "t5", Source: tested, Type: mgen.Boolean, Line: 5}},
	}

	mutagen.EXPECT().
		StreamMutations(mock.Anything, mock.Anything, mock.Anything, typeArgs(domain.DefaultMutations)...).
		RunAndReturn(func(ctx context.Context, source rpt.Source, fn func(m.Mutation) error, types ...mgen.Type) error {
			return streamMutationsFn(byPath[source.Origin.FullPath])(ctx, source, fn, types...)
		}).Maybe()

	reporter.EXPECT().StartTest(mock.Anything).Return(nil).Maybe()
	reporter.EXPECT().Wait(mock.Anything).Return().Maybe()
	reporter.EXPECT().Close(mock.Anything).Return().Maybe()
	reporter.EXPECT().DisplayConcurrencyInfo(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return().Maybe()
	reporter.EXPECT().DisplayUpcomingTestsInfo(mock.Anything, mock.Anything).Return().Maybe()
	reporter.EXPECT().DisplayStartingTestInfo(mock.Anything, mock.Anything, mock.Anything).Return().Maybe()
	reporter.EXPECT().DisplayCompletedTestInfo(mock.Anything, mock.Anything, mock.Anything).Return().Maybe()
	reporter.EXPECT().DisplayMutationScore(mock.Anything, mock.Anything).Return().Maybe()

	workspace.EXPECT().Run(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, mutation m.Mutation) (m.Result, error) {
		f.mu.Lock()
		f.tested = append(f.tested, mutation.ID)
		f.mu.Unlock()

		return m.Result{mutation.Type: {{MutationID: mutation.ID, Status: rpt.Survived}}}, nil
	}).Maybe()
	workspace.EXPECT().Close(mock.Anything).Return().Maybe()
	orchestrator.EXPECT().NewWorkspace(mock.Anything).Return(workspace).Maybe()

	f.store.EXPECT().CleanReports(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, _ m.Path, sources pkg.FileSpill[rpt.Source]) error {
		return sources.Range(func(_ uint64, source rpt.Source) error {
			f.cleaned = append(f.cleaned, source.Origin.ShortPath)
			return nil
		})
	}).Maybe()
	f.store.EXPECT().SaveReports(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, _ m.Path, reports pkg.FileSpill[m.Report]) error {
		return reports.Range(func(_ uint64, report m.Report) error {
			f.saved = append(f.saved, report)
			return nil
		})
	}).Maybe()
	f.store.EXPECT().RegenerateIndex(mock.Anything, mock.Anything).Return(nil).Maybe()
	f.store.EXPECT().SaveSelection(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, _ m.Path, sel rpt.Selection) error {
		f.selection = &sel
		return nil
	}).Maybe()

	f.wf = domain.NewWorkflow(f.fs, f.store, reporter, orchestrator, mutagen, domain.WithChangeDetector(f.changes))

	return f
}

func diffTestArgs(shard, total int) domain.TestArgs {
	return domain.TestArgs{
		EstimateArgs:    domain.EstimateArgs{Paths: []m.Path{"./..."}, Diff: "origin/main"},
		Reports:         ".gooze-reports",
		Threads:         2,
		ShardIndex:      shard,
		TotalShardCount: total,
	}
}

func TestWorkflow_Test_DiffTestsOnlyChangedLines(t *testing.T) {
	f := newDiffRunFixture(t)
	f.changes.EXPECT().Changes(mock.Anything, m.Path("."), "origin/main").Return(diffChanges, nil).Once()

	require.NoError(t, f.wf.Test(context.Background(), diffTestArgs(0, 1)))

	assert.ElementsMatch(t, []string{"c3a", "c3b", "t5"}, f.tested, "calc.go line 3 and all of tested.go")
	// Only the sources a diff run tests lose their stored reports; same.go keeps its own.
	assert.ElementsMatch(t, []string{"calc.go", "tested.go"}, f.cleaned)
	assert.Equal(t, &rpt.Selection{Diff: "origin/main", Base: "1a2b3c4d"}, f.selection,
		"the run records that it is a diff run, for report view's diff thresholds")

	require.Len(t, f.saved, 3)

	for _, report := range f.saved {
		switch report.Source.Origin.ShortPath {
		case "calc.go":
			assert.Equal(t, 3, report.Line)
			assert.Equal(t, &rpt.Scope{Base: "1a2b3c4d", Lines: []rpt.LineRange{{Start: 3, End: 3}}}, report.Scope,
				"calc.go was tested on line 3 only")
		case "tested.go":
			assert.Equal(t, 5, report.Line)
			assert.Nil(t, report.Scope, "a changed test puts tested.go in scope in full")
		default:
			t.Errorf("unexpected report for %s", report.Source.Origin.ShortPath)
		}
	}
}

func TestWorkflow_Test_DiffShardsSplitTheSameMutations(t *testing.T) {
	var all []string

	for shard := range 2 {
		f := newDiffRunFixture(t)
		f.changes.EXPECT().Changes(mock.Anything, mock.Anything, "origin/main").Return(diffChanges, nil).Once()

		require.NoError(t, f.wf.Test(context.Background(), diffTestArgs(shard, 2)))

		all = append(all, f.tested...)
	}

	assert.ElementsMatch(t, []string{"c3a", "c3b", "t5"}, all, "each in-scope mutation runs in exactly one shard")
}

func TestWorkflow_Test_WithoutDiffTestsEverything(t *testing.T) {
	f := newDiffRunFixture(t)
	args := diffTestArgs(0, 1)
	args.Diff = ""

	require.NoError(t, f.wf.Test(context.Background(), args))

	assert.ElementsMatch(t, []string{"c3a", "c3b", "c7", "s2", "t5"}, f.tested)

	for _, report := range f.saved {
		assert.Nil(t, report.Scope, "a full run records no scope")
		assert.NotZero(t, report.Line, "every report records its line")
	}

	f.changes.AssertNotCalled(t, "Changes", mock.Anything, mock.Anything, mock.Anything)
	assert.Nil(t, f.selection, "a full run records no selection")
}

func TestWorkflow_Test_DiffErrorStopsTheRun(t *testing.T) {
	f := newDiffRunFixture(t)
	f.changes.EXPECT().Changes(mock.Anything, mock.Anything, "origin/main").Return(domain.ChangeSet{}, domain.ErrUnknownRef).Once()

	err := f.wf.Test(context.Background(), diffTestArgs(0, 1))
	require.ErrorIs(t, err, domain.ErrUnknownRef)
	assert.Empty(t, f.tested)
	f.fs.AssertNotCalled(t, "Stream", mock.Anything, mock.Anything)
}
