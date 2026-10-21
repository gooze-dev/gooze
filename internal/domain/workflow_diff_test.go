package domain_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	domain "gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
	m "gooze.dev/pkg/gooze/internal/model"
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
		StreamMutations(mock.Anything, mock.Anything, mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4], domain.DefaultMutations[5]).
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
