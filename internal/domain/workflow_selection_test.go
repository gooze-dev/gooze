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
		})
	}
}
