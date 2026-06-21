package domain_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	adaptermocks "gooze.dev/pkg/gooze/internal/adapter/mocks"
	"gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
	m "gooze.dev/pkg/gooze/internal/model"
	pkg "gooze.dev/pkg/gooze/pkg"
)

func TestWorkflow_Test_CoverageGateSkipsUncovered(t *testing.T) {
	ctx := context.Background()

	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReporter := new(domainmocks.MockReporter)
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockMutagen := new(domainmocks.MockMutagen)

	source := m.Source{
		Origin: &m.File{FullPath: "pkg/foo.go", ShortPath: "pkg/foo.go", Hash: "h"},
		Test:   &m.File{FullPath: "pkg/foo_test.go", Hash: "th"},
	}

	covered := m.Mutation{ID: "covered", Source: source, Type: m.MutationArithmetic, Line: 10}
	uncovered := m.Mutation{ID: "uncovered", Source: source, Type: m.MutationArithmetic, Line: 20}

	profile := []byte("mode: set\nmod/pkg/foo.go:10.1,10.20 1 1\n")

	mockReporter.EXPECT().StartTest(ctx).Return(nil).Once()
	mockReporter.EXPECT().Wait(ctx).Return().Once()
	mockReporter.EXPECT().Close(ctx).Return().Once()
	mockReporter.EXPECT().DisplayConcurrencyInfo(ctx, mock.Anything, mock.Anything, mock.Anything).Return()
	mockReporter.EXPECT().DisplayUpcomingTestsInfo(ctx, mock.Anything).Return()
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockReporter.EXPECT().DisplayStartingTestInfo(ctx, mock.Anything, mock.Anything).Return().Maybe()
	mockReporter.EXPECT().DisplayCompletedTestInfo(ctx, mock.Anything, mock.Anything).Return().Maybe()

	mockFSAdapter.EXPECT().Stream(ctx, mock.Anything).Return(streamSources([]m.Source{source}))
	mockFSAdapter.EXPECT().ReadFile(ctx, m.Path("cov.out")).Return(profile, nil)

	mockMutagen.EXPECT().
		StreamMutations(ctx, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(streamMutationsFn([]m.Mutation{covered, uncovered}))

	// Only the covered mutation reaches a workspace.
	mockOrchestrator.EXPECT().NewWorkspace().Return(mockWorkspace).Maybe()
	mockWorkspace.EXPECT().Close(mock.Anything).Return().Maybe()
	mockWorkspace.EXPECT().
		Run(mock.Anything, mock.MatchedBy(func(mut m.Mutation) bool { return mut.ID == "covered" })).
		Return(m.Result{m.MutationArithmetic: {{MutationID: "covered", Status: m.Killed}}}, nil).
		Once()

	var saved []m.Report
	mockReportStore.EXPECT().SaveSpillReports(ctx, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _ m.Path, reports pkg.FileSpill[m.Report]) error {
			saved = collectSpillReports(t, reports)
			return nil
		})
	mockReportStore.EXPECT().RegenerateIndex(ctx, mock.Anything).Return(nil).Maybe()

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockReporter, mockOrchestrator, mockMutagen)

	err := wf.Test(ctx, domain.TestArgs{
		EstimateArgs:    domain.EstimateArgs{Paths: []m.Path{"pkg"}},
		Reports:         "reports",
		Threads:         1,
		TotalShardCount: 1,
		CoverageProfile: "cov.out",
	})
	require.NoError(t, err)

	statusByID := map[string]m.TestStatus{}
	for _, r := range saved {
		for _, entries := range r.Result {
			for _, e := range entries {
				statusByID[e.MutationID] = e.Status
			}
		}
	}

	assert.Equal(t, m.Killed, statusByID["covered"])
	assert.Equal(t, m.NotCovered, statusByID["uncovered"], "uncovered mutation should be reported NotCovered without running tests")

	mockWorkspace.AssertExpectations(t)
	mockMutagen.AssertExpectations(t)
}
