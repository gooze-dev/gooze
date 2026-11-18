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

func TestWorkflow_Test_CoverageGateSkipsUncovered(t *testing.T) {
	ctx := context.Background()

	mockFSAdapter := new(domainmocks.MockSourceFSAdapter)
	mockReportStore := new(domainmocks.MockReportStore)
	mockReportStore.EXPECT().CleanReports(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockMutagen := new(domainmocks.MockMutagen)

	source := rpt.Source{
		Origin: &rpt.File{FullPath: "pkg/foo.go", ShortPath: "pkg/foo.go", Hash: "h"},
		Test:   &rpt.File{FullPath: "pkg/foo_test.go", Hash: "th"},
	}

	covered := m.Mutation{ID: "covered", Source: source, Type: mgen.Arithmetic, Line: 10}
	uncovered := m.Mutation{ID: "uncovered", Source: source, Type: mgen.Arithmetic, Line: 20}

	profile := []byte("mode: set\nmod/pkg/foo.go:10.1,10.20 1 1\nmod/pkg/foo.go:20.1,20.20 1 0\n")

	mockReporter.EXPECT().StartTest(ctx).Return(nil).Once()
	mockReporter.EXPECT().Wait(ctx).Return().Once()
	mockReporter.EXPECT().Close(ctx).Return().Once()
	mockReporter.EXPECT().DisplayConcurrencyInfo(ctx, mock.Anything, mock.Anything, mock.Anything).Return()
	mockReporter.EXPECT().DisplayUpcomingTestsInfo(ctx, mock.Anything).Return()
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockReporter.EXPECT().DisplayStartingTestInfo(ctx, mock.Anything, mock.Anything).Return().Maybe()
	mockReporter.EXPECT().DisplayCompletedTestInfo(ctx, mock.Anything, mock.Anything).Return().Maybe()

	mockFSAdapter.EXPECT().Stream(ctx, mock.Anything).Return(streamSources([]rpt.Source{source}))
	mockFSAdapter.EXPECT().ReadFile(ctx, m.Path("cov.out")).Return(profile, nil)

	mockMutagen.EXPECT().
		StreamMutations(ctx, mock.Anything, mock.Anything, typeArgs(domain.DefaultMutations)...).
		RunAndReturn(streamMutationsFn([]m.Mutation{covered, uncovered}))

	// Only the covered mutation reaches a workspace.
	mockOrchestrator.EXPECT().NewWorkspace(mock.Anything).Return(mockWorkspace).Maybe()
	mockWorkspace.EXPECT().Close(mock.Anything).Return().Maybe()
	mockWorkspace.EXPECT().
		Run(mock.Anything, mock.MatchedBy(func(mut m.Mutation) bool { return mut.ID == "covered" })).
		Return(m.Result{mgen.Arithmetic: {{MutationID: "covered", Status: rpt.Killed}}}, nil).
		Once()

	var saved []m.Report
	mockReportStore.EXPECT().SaveReports(ctx, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _ m.Path, reports pkg.FileSpill[m.Report]) error {
			saved = collectSpillReports(t, reports)
			return nil
		})
	mockReportStore.EXPECT().RegenerateIndex(ctx, mock.Anything).Return(nil).Maybe()
	mockReportStore.EXPECT().SavePartial(ctx, mock.Anything, rpt.Partial{}).Return(nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockReporter, mockOrchestrator, mockMutagen)

	err := wf.Test(ctx, domain.TestArgs{
		Paths:           []m.Path{"pkg"},
		Reports:         "reports",
		Threads:         1,
		TotalShardCount: 1,
		CoverageProfile: "cov.out",
	})
	require.NoError(t, err)

	statusByID := map[string]rpt.TestStatus{}
	for _, r := range saved {
		for _, entries := range r.Result {
			for _, e := range entries {
				statusByID[e.MutationID] = e.Status
			}
		}
	}

	assert.Equal(t, rpt.Killed, statusByID["covered"])
	assert.Equal(t, rpt.NotCovered, statusByID["uncovered"], "uncovered mutation should be reported NotCovered without running tests")

	mockWorkspace.AssertExpectations(t)
	mockMutagen.AssertExpectations(t)
}

// coverageStatuses runs the mutations of pkg/foo.go under profile and returns
// the status each one is reported with; the tests kill every mutant they run.
func coverageStatuses(t *testing.T, profile string, conditions bool, mutations ...m.Mutation) map[string]rpt.TestStatus {
	t.Helper()

	ctx := context.Background()

	mockFSAdapter := new(domainmocks.MockSourceFSAdapter)
	mockReportStore := new(domainmocks.MockReportStore)
	mockReportStore.EXPECT().CleanReports(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockMutagen := new(domainmocks.MockMutagen)

	source := rpt.Source{
		Origin: &rpt.File{FullPath: "pkg/foo.go", ShortPath: "pkg/foo.go", Hash: "h"},
		Test:   &rpt.File{FullPath: "pkg/foo_test.go", Hash: "th"},
	}

	for i := range mutations {
		mutations[i].Source = source
		mutations[i].Type = mgen.Comparison
	}

	mockReporter.EXPECT().StartTest(ctx).Return(nil).Once()
	mockReporter.EXPECT().Wait(ctx).Return().Once()
	mockReporter.EXPECT().Close(ctx).Return().Once()
	mockReporter.EXPECT().DisplayConcurrencyInfo(ctx, mock.Anything, mock.Anything, mock.Anything).Return()
	mockReporter.EXPECT().DisplayUpcomingTestsInfo(ctx, mock.Anything).Return()
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockReporter.EXPECT().DisplayStartingTestInfo(ctx, mock.Anything, mock.Anything).Return().Maybe()
	mockReporter.EXPECT().DisplayCompletedTestInfo(ctx, mock.Anything, mock.Anything).Return().Maybe()

	mockFSAdapter.EXPECT().Stream(ctx, mock.Anything).Return(streamSources([]rpt.Source{source}))
	mockFSAdapter.EXPECT().ReadFile(ctx, m.Path("cov.out")).Return([]byte(profile), nil)

	mockMutagen.EXPECT().
		StreamMutations(ctx, mock.Anything, mock.Anything, typeArgs(domain.DefaultMutations)...).
		RunAndReturn(streamMutationsFn(mutations))

	mockOrchestrator.EXPECT().NewWorkspace(mock.Anything).Return(mockWorkspace).Maybe()
	mockWorkspace.EXPECT().Close(mock.Anything).Return().Maybe()
	mockWorkspace.EXPECT().Run(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, mutation m.Mutation) (m.Result, error) {
			return m.Result{mgen.Comparison: {{MutationID: mutation.ID, Status: rpt.Killed}}}, nil
		}).Maybe()

	var saved []m.Report

	mockReportStore.EXPECT().SaveReports(ctx, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _ m.Path, reports pkg.FileSpill[m.Report]) error {
			saved = collectSpillReports(t, reports)
			return nil
		})
	mockReportStore.EXPECT().RegenerateIndex(ctx, mock.Anything).Return(nil).Maybe()
	mockReportStore.EXPECT().SavePartial(ctx, mock.Anything, rpt.Partial{}).Return(nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockReporter, mockOrchestrator, mockMutagen)

	require.NoError(t, wf.Test(ctx, domain.TestArgs{
		EstimateArgs:       domain.EstimateArgs{Paths: []m.Path{"pkg"}},
		Reports:            "reports",
		Threads:            1,
		TotalShardCount:    1,
		CoverageProfile:    "cov.out",
		CoverageConditions: conditions,
	}))

	statuses := map[string]rpt.TestStatus{}

	for _, report := range saved {
		for _, entries := range report.Result {
			for _, entry := range entries {
				statuses[entry.MutationID] = entry.Status
			}
		}
	}

	return statuses
}

// The profile of
//
//	10	if a < b {
//	11		return a
//	12	}
//	13	if a > b {
//	14		return b
//	15	}
//
// when a < b never held and a > b did.
const conditionsProfile = "mode: set\nmod/pkg/foo.go:10.2,10.11 1 3\nmod/pkg/foo.go:10.11,12.3 1 0\n" +
	"mod/pkg/foo.go:13.2,13.11 1 3\nmod/pkg/foo.go:13.11,15.3 1 3\n"

func conditionMutations() []m.Mutation {
	return []m.Mutation{
		{ID: "never-entered", Line: 10, Guards: &m.Position{Line: 11, Column: 3}},
		{ID: "entered", Line: 13, Guards: &m.Position{Line: 14, Column: 3}},
		{ID: "no-condition", Line: 13},
	}
}

func TestWorkflow_Test_ConditionOfBlockThatNeverRanIsNotCovered(t *testing.T) {
	statuses := coverageStatuses(t, conditionsProfile, true, conditionMutations()...)

	assert.Equal(t, map[string]rpt.TestStatus{
		"never-entered": rpt.NotCovered,
		"entered":       rpt.Killed,
		"no-condition":  rpt.Killed,
	}, statuses)
}

func TestWorkflow_Test_CoverageConditionsOffRunsThem(t *testing.T) {
	statuses := coverageStatuses(t, conditionsProfile, false, conditionMutations()...)

	assert.Equal(t, map[string]rpt.TestStatus{
		"never-entered": rpt.Killed,
		"entered":       rpt.Killed,
		"no-condition":  rpt.Killed,
	}, statuses)
}
