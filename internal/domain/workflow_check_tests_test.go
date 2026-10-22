package domain_test

import (
	"context"
	"testing"
	"time"

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

// checkingWorkspace is a Workspace that is also a TestChecker, as gooze's own
// workspaces are. Its mocks panic on an unexpected call from a worker instead
// of failing the test from that goroutine, which would hang the run.
type checkingWorkspace struct {
	*domainmocks.MockWorkspace
	*domainmocks.MockTestChecker
}

func newCheckingWorkspace(t *testing.T) checkingWorkspace {
	t.Helper()

	ws := checkingWorkspace{new(domainmocks.MockWorkspace), new(domainmocks.MockTestChecker)}
	ws.MockWorkspace.EXPECT().Close(mock.Anything).Return().Maybe()

	t.Cleanup(func() {
		ws.MockWorkspace.AssertExpectations(t)
		ws.MockTestChecker.AssertExpectations(t)
	})

	return ws
}

func checkSource(dir, name string) rpt.Source {
	return rpt.Source{
		Origin: &rpt.File{FullPath: "/p/" + dir + "/" + name + ".go", ShortPath: dir + "/" + name + ".go", Hash: dir + name},
		Test:   &rpt.File{FullPath: "/p/" + dir + "/" + name + "_test.go", ShortPath: dir + "/" + name + "_test.go", Hash: dir + name + "t"},
	}
}

// runChecked runs a workflow over mutations with ws as every worker's
// workspace and returns its error and the short paths of the saved reports.
func runChecked(t *testing.T, ctx context.Context, ws domain.Workspace, mutations []m.Mutation, args domain.TestArgs) (error, []string) {
	t.Helper()

	mockFSAdapter := domainmocks.NewMockSourceFSAdapter(t)
	mockReportStore := domainmocks.NewMockReportStore(t)
	mockReporter := new(domainmocks.MockReporter)
	mockOrchestrator := domainmocks.NewMockOrchestrator(t)
	mockMutagen := domainmocks.NewMockMutagen(t)

	var sources []rpt.Source

	for _, mutation := range mutations {
		if len(sources) == 0 || sources[len(sources)-1].Origin.FullPath != mutation.Source.Origin.FullPath {
			sources = append(sources, mutation.Source)
		}
	}

	mockReporter.EXPECT().StartTest(ctx).Return(nil)
	mockReporter.EXPECT().Wait(ctx).Return().Maybe()
	mockReporter.EXPECT().Close(ctx).Return()
	mockReporter.EXPECT().DisplayConcurrencyInfo(ctx, mock.Anything, mock.Anything, mock.Anything).Return()
	mockReporter.EXPECT().DisplayUpcomingTestsInfo(ctx, mock.Anything).Return()
	mockReporter.EXPECT().DisplayStartingTestInfo(ctx, mock.Anything, mock.Anything).Return().Maybe()
	mockReporter.EXPECT().DisplayCompletedTestInfo(ctx, mock.Anything, mock.Anything).Return().Maybe()
	mockReporter.EXPECT().DisplayMutationScore(mock.Anything, mock.Anything).Return().Maybe()
	mockFSAdapter.EXPECT().Stream(ctx, mock.Anything).Return(streamSources(sources))
	mockMutagen.EXPECT().
		StreamMutations(ctx, mock.Anything, mock.Anything, typeArgs(domain.DefaultMutations)...).
		RunAndReturn(func(_ context.Context, source rpt.Source, fn func(m.Mutation) error, _ ...mgen.Type) error {
			for _, mutation := range mutations {
				if mutation.Source.Origin.FullPath == source.Origin.FullPath {
					if err := fn(mutation); err != nil {
						return err
					}
				}
			}

			return nil
		})
	mockOrchestrator.EXPECT().NewWorkspace(mock.Anything).Return(ws)

	var saved []string

	// An interrupted run saves and scores on a context without the
	// cancellation, so these take any context.
	mockReportStore.EXPECT().CleanReports(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReportStore.EXPECT().SaveReports(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _ m.Path, reports pkg.FileSpill[m.Report]) error {
			return reports.Range(func(_ uint64, report m.Report) error {
				saved = append(saved, report.Source.Origin.ShortPath)
				return nil
			})
		}).Maybe()
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything, mock.Anything).Return(nil).Maybe()

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockReporter, mockOrchestrator, mockMutagen)

	args.Paths = []m.Path{"./..."}
	args.Reports = "reports"
	args.Threads = 1
	args.TotalShardCount = 1

	err := wf.Test(ctx, args)

	return err, saved
}

func killedResult(mutation m.Mutation) m.Result {
	return m.Result{mutation.Type: {{MutationID: mutation.ID, Status: rpt.Killed}}}
}

// A package whose tests fail without any mutation would score every mutant as
// killed. Its mutants are not tested or saved, the other packages still are,
// and the run ends with ExitTestsFail. Each package is checked once.
func TestWorkflow_Test_SkipsPackagesWhoseTestsFail(t *testing.T) {
	ctx := context.Background()
	broken, flaky, healthy := checkSource("broken", "max"), checkSource("flaky", "net"), checkSource("ok", "min")
	mutations := []m.Mutation{
		{ID: "b-1", Source: broken, Type: mgen.Comparison},
		{ID: "b-2", Source: broken, Type: mgen.Comparison},
		{ID: "f-1", Source: flaky, Type: mgen.Comparison},
		{ID: "o-1", Source: healthy, Type: mgen.Comparison},
	}

	ws := newCheckingWorkspace(t)
	ws.MockTestChecker.EXPECT().CheckTests(mock.Anything, mutations[0]).Return(&domain.FailingTestsError{
		Package: "./broken",
		Output:  "=== RUN   TestMax\n    max_test.go:5: this test always fails\n--- FAIL: TestMax (0.00s)\nFAIL\n",
	}).Once()
	ws.MockTestChecker.EXPECT().CheckTests(mock.Anything, mutations[2]).
		Return(&domain.FailingTestsError{Package: "./flaky", TimedOut: true}).Once()
	ws.MockTestChecker.EXPECT().CheckTests(mock.Anything, mutations[3]).Return(nil).Once()
	ws.MockWorkspace.EXPECT().Run(mock.Anything, mutations[3]).Return(killedResult(mutations[3]), nil).Once()

	err, saved := runChecked(t, ctx, ws, mutations, domain.TestArgs{})

	var coded *m.CodedError
	require.ErrorAs(t, err, &coded)
	assert.Equal(t, m.ExitTestsFail, coded.Code)
	assert.Equal(t, `tests fail without any mutation in 2 packages, so 3 mutants were not tested:

tests of ./broken fail without any mutation (2 mutants)
    === RUN   TestMax
        max_test.go:5: this test always fails
    --- FAIL: TestMax (0.00s)
    FAIL

tests of ./flaky don't finish within the mutation timeout without any mutation (1 mutant)

Fix these tests (go test should pass), then run gooze again.`, err.Error())
	assert.Equal(t, []string{"ok/min.go"}, saved)
}

// The check runs under the mutation timeout, when one is set.
func TestWorkflow_Test_ChecksTestsUnderTheMutationTimeout(t *testing.T) {
	for name, tc := range map[string]struct {
		timeout  time.Duration
		deadline bool
	}{
		"1ns":      {timeout: 1, deadline: true},
		"none":     {timeout: 0},
		"negative": {timeout: -1},
	} {
		t.Run(name, func(t *testing.T) {
			mutation := m.Mutation{ID: "o-1", Source: checkSource("ok", "min"), Type: mgen.Comparison}

			ws := newCheckingWorkspace(t)
			ws.MockTestChecker.EXPECT().CheckTests(mock.Anything, mutation).
				RunAndReturn(func(ctx context.Context, _ m.Mutation) error {
					_, ok := ctx.Deadline()
					assert.Equal(t, tc.deadline, ok)

					return nil
				}).Once()
			ws.MockWorkspace.EXPECT().Run(mock.Anything, mutation).Return(killedResult(mutation), nil).Once()

			err, _ := runChecked(t, context.Background(), ws, []m.Mutation{mutation}, domain.TestArgs{MutationTimeout: tc.timeout})
			require.NoError(t, err)
		})
	}
}

// A source without tests, or a workspace that can't check tests, goes
// straight to Run.
func TestWorkflow_Test_ChecksOnlySourcesWithTests(t *testing.T) {
	untested := checkSource("ok", "min")
	untested.Test = nil
	mutation := m.Mutation{ID: "o-1", Source: untested, Type: mgen.Comparison}

	ws := newCheckingWorkspace(t)
	ws.MockWorkspace.EXPECT().Run(mock.Anything, mutation).Return(killedResult(mutation), nil).Once()

	err, _ := runChecked(t, context.Background(), ws, []m.Mutation{mutation}, domain.TestArgs{})
	require.NoError(t, err)

	plain := new(domainmocks.MockWorkspace)
	plain.EXPECT().Close(mock.Anything).Return().Maybe()
	defer plain.AssertExpectations(t)
	tested := m.Mutation{ID: "o-2", Source: checkSource("ok", "min"), Type: mgen.Comparison}
	plain.EXPECT().Run(mock.Anything, tested).Return(killedResult(tested), nil).Once()

	err, _ = runChecked(t, context.Background(), plain, []m.Mutation{tested}, domain.TestArgs{})
	require.NoError(t, err)
}

// A check cut short by the run ending isn't a failing package: the mutant
// goes on to Run, which reports the cancellation.
func TestWorkflow_Test_CanceledCheckIsNotAFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mutation := m.Mutation{ID: "o-1", Source: checkSource("ok", "min"), Type: mgen.Comparison}

	ws := newCheckingWorkspace(t)
	ws.MockTestChecker.EXPECT().CheckTests(mock.Anything, mutation).
		RunAndReturn(func(context.Context, m.Mutation) error {
			cancel()
			return &domain.FailingTestsError{Package: "./ok", TimedOut: true}
		}).Once()
	ws.MockWorkspace.EXPECT().Run(mock.Anything, mutation).
		Return(m.Result{mutation.Type: {{MutationID: mutation.ID, Status: rpt.Timeout}}}, nil).Once()

	err, _ := runChecked(t, ctx, ws, []m.Mutation{mutation}, domain.TestArgs{})

	var failing *domain.FailingTestsError
	assert.NotErrorAs(t, err, &failing)
	assert.ErrorIs(t, err, context.Canceled, "the run ends as an interrupted one")
}

// Failing tests outrank a breached threshold: the score leaves out the
// packages whose mutants weren't tested.
func TestWorkflow_Test_FailingTestsOutrankAThreshold(t *testing.T) {
	broken, healthy := checkSource("broken", "max"), checkSource("ok", "min")
	mutations := []m.Mutation{
		{ID: "b-1", Source: broken, Type: mgen.Comparison},
		{ID: "o-1", Source: healthy, Type: mgen.Comparison},
	}

	ws := newCheckingWorkspace(t)
	ws.MockTestChecker.EXPECT().CheckTests(mock.Anything, mutations[0]).
		Return(&domain.FailingTestsError{Package: "./broken"}).Once()
	ws.MockTestChecker.EXPECT().CheckTests(mock.Anything, mutations[1]).Return(nil).Once()
	ws.MockWorkspace.EXPECT().Run(mock.Anything, mutations[1]).
		Return(m.Result{mutations[1].Type: {{MutationID: mutations[1].ID, Status: rpt.Survived}}}, nil).Once()

	err, _ := runChecked(t, context.Background(), ws, mutations, domain.TestArgs{Thresholds: domain.Thresholds{Total: 100}})

	var coded *m.CodedError
	require.ErrorAs(t, err, &coded)
	assert.Equal(t, m.ExitTestsFail, coded.Code)
}
