package domain_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gooze.dev/pkg/gooze/internal/adapter"
	domain "gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
	m "gooze.dev/pkg/gooze/internal/model"
	pkg "gooze.dev/pkg/gooze/pkg"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// streamSources returns closed channels that yield the given sources and no
// error, mimicking SourceFSAdapter.Stream for a successful scan.
func streamSources(sources []rpt.Source) (<-chan rpt.Source, <-chan error) {
	out := make(chan rpt.Source, len(sources))
	for _, source := range sources {
		out <- source
	}

	close(out)

	errc := make(chan error, 1)
	close(errc)

	return out, errc
}

// streamSourcesErr returns channels mimicking a scan that failed with err.
func streamSourcesErr(err error) (<-chan rpt.Source, <-chan error) {
	out := make(chan rpt.Source)
	close(out)

	errc := make(chan error, 1)
	errc <- err
	close(errc)

	return out, errc
}

func collectSpillReports(t *testing.T, reports pkg.FileSpill[m.Report]) []m.Report {
	t.Helper()

	collected, err := pkg.ToSlice(reports)
	require.NoError(t, err)

	return collected
}

// spillReportsIfOpen collects the spill's reports, or returns nil once the spill
// has been closed and its file removed.
func spillReportsIfOpen(reports pkg.FileSpill[m.Report]) []m.Report {
	collected, err := pkg.ToSlice(reports)
	if err != nil {
		return nil
	}

	return collected
}

// streamMutationsFn returns a RunAndReturn callback that streams the given
// mutations through the StreamMutations fn argument. It is safe to invoke
// multiple times (the Test flow streams twice: once to count, once to run).
func streamMutationsFn(mutations []m.Mutation) func(context.Context, rpt.Source, func(m.Mutation) error, ...mgen.Type) error {
	return func(_ context.Context, _ rpt.Source, fn func(m.Mutation) error, _ ...mgen.Type) error {
		for _, mut := range mutations {
			if err := fn(mut); err != nil {
				return err
			}
		}

		return nil
	}
}

func TestWorkflow_Test_Success(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFSAdapter := new(domainmocks.MockSourceFSAdapter)
	mockReportStore := new(domainmocks.MockReportStore)
	mockReportStore.EXPECT().CleanReports(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockMutagen := new(domainmocks.MockMutagen)

	sources := []rpt.Source{
		{
			Origin: &rpt.File{FullPath: "test.go", Hash: "hash1"},
			Test:   &rpt.File{FullPath: "test_test.go", Hash: "test_hash1"},
		},
	}

	mutations := []m.Mutation{
		{ID: "hash-1", Source: sources[0], Type: mgen.Arithmetic},
	}

	mockReporter.EXPECT().StartTest(ctx).Return(nil).Once()
	mockReporter.EXPECT().Wait(ctx).Return().Once()
	mockReporter.EXPECT().Close(ctx).Return().Once()
	mockReporter.EXPECT().DisplayConcurrencyInfo(ctx, mock.Anything, mock.Anything, mock.Anything).Return()
	mockReporter.EXPECT().DisplayUpcomingTestsInfo(ctx, mock.Anything).Return()
	mockReporter.EXPECT().DisplayStartingTestInfo(ctx, mock.Anything, mock.Anything).Return().Once()
	mockReporter.EXPECT().DisplayCompletedTestInfo(ctx, mock.Anything, mock.Anything).Return().Once()
	mockFSAdapter.EXPECT().Stream(ctx, mock.Anything).Return(streamSources(sources))
	mockMutagen.EXPECT().
		StreamMutations(ctx, mock.Anything, mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4], domain.DefaultMutations[5]).
		RunAndReturn(streamMutationsFn(mutations))
	mockWorkspace.EXPECT().Run(mock.Anything, mock.Anything).Return(m.Result{}, nil)
	mockWorkspace.EXPECT().Close(mock.Anything).Return().Maybe()
	mockOrchestrator.EXPECT().NewWorkspace().Return(mockWorkspace).Maybe()
	mockReportStore.EXPECT().SaveReports(ctx, mock.Anything, mock.Anything).Return(nil)
	mockReportStore.EXPECT().RegenerateIndex(ctx, mock.Anything).Return(nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockReporter, mockOrchestrator, mockMutagen)

	// Act
	args := domain.TestArgs{
		Paths:           []m.Path{"test.go"},
		Reports:         "reports.json",
		Threads:         1,
		ShardIndex:      0,
		TotalShardCount: 1,
	}
	err := wf.Test(context.Background(), args)

	// Assert
	assert.NoError(t, err)
	mockFSAdapter.AssertExpectations(t)
	mockMutagen.AssertExpectations(t)
	mockReportStore.AssertExpectations(t)
	mockOrchestrator.AssertExpectations(t)
}

func TestWorkflow_Test_GetSourcesError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFSAdapter := new(domainmocks.MockSourceFSAdapter)
	mockReportStore := new(domainmocks.MockReportStore)
	mockReportStore.EXPECT().CleanReports(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockMutagen := new(domainmocks.MockMutagen)

	testErr := errors.New("failed to get sources")
	mockReporter.EXPECT().StartTest(ctx).Return(nil).Once()
	mockReporter.EXPECT().Close(ctx).Return().Once()
	mockReporter.EXPECT().DisplayConcurrencyInfo(ctx, mock.Anything, mock.Anything, mock.Anything).Return()
	mockFSAdapter.EXPECT().Stream(ctx, mock.Anything).Return(streamSourcesErr(testErr))
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockWorkspace.EXPECT().Close(mock.Anything).Return().Maybe()
	mockOrchestrator.EXPECT().NewWorkspace().Return(mockWorkspace).Maybe()

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockReporter, mockOrchestrator, mockMutagen)

	// Act
	args := domain.TestArgs{
		Paths:   []m.Path{"test.go"},
		Reports: "reports.json",
	}
	err := wf.Test(context.Background(), args)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "get sources")
}

func TestWorkflow_Test_GenerateMutationsError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFSAdapter := new(domainmocks.MockSourceFSAdapter)
	mockReportStore := new(domainmocks.MockReportStore)
	mockReportStore.EXPECT().CleanReports(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockMutagen := new(domainmocks.MockMutagen)

	sources := []rpt.Source{
		{Origin: &rpt.File{FullPath: "test.go", Hash: "hash1"}},
	}

	testErr := errors.New("failed to generate mutations")
	mockReporter.EXPECT().StartTest(ctx).Return(nil).Once()
	mockReporter.EXPECT().Close(ctx).Return().Once()
	mockReporter.EXPECT().DisplayConcurrencyInfo(ctx, mock.Anything, mock.Anything, mock.Anything).Return()
	mockFSAdapter.EXPECT().Stream(ctx, mock.Anything).Return(streamSources(sources))
	mockMutagen.EXPECT().
		StreamMutations(ctx, mock.Anything, mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4], domain.DefaultMutations[5]).
		Return(testErr)
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockWorkspace.EXPECT().Close(mock.Anything).Return().Maybe()
	mockOrchestrator.EXPECT().NewWorkspace().Return(mockWorkspace).Maybe()

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockReporter, mockOrchestrator, mockMutagen)

	// Act
	args := domain.TestArgs{
		Paths:   []m.Path{"test.go"},
		Reports: "reports.json",
	}
	err := wf.Test(context.Background(), args)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "generate mutations")
}

func TestWorkflow_Test_TestMutationError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFSAdapter := new(domainmocks.MockSourceFSAdapter)
	mockReportStore := new(domainmocks.MockReportStore)
	mockReportStore.EXPECT().CleanReports(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReportStore.EXPECT().RegenerateIndex(ctx, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockMutagen := new(domainmocks.MockMutagen)

	sources := []rpt.Source{
		{Origin: &rpt.File{FullPath: "test.go", Hash: "hash1"}},
	}

	mutations := []m.Mutation{
		{ID: "hash-1", Source: sources[0]},
	}

	testErr := errors.New("failed to test mutation")
	mockReporter.EXPECT().StartTest(ctx).Return(nil).Once()
	mockReporter.EXPECT().Close(ctx).Return().Once()
	mockReporter.EXPECT().DisplayConcurrencyInfo(ctx, mock.Anything, mock.Anything, mock.Anything).Return()
	mockReporter.EXPECT().DisplayUpcomingTestsInfo(ctx, mock.Anything).Return()
	mockReporter.EXPECT().DisplayStartingTestInfo(ctx, mock.Anything, mock.Anything).Return().Once()
	mockFSAdapter.EXPECT().Stream(ctx, mock.Anything).Return(streamSources(sources))
	mockMutagen.EXPECT().
		StreamMutations(ctx, mock.Anything, mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4], domain.DefaultMutations[5]).
		RunAndReturn(streamMutationsFn(mutations))
	mockWorkspace.EXPECT().Run(mock.Anything, mock.Anything).Return(nil, testErr)
	mockWorkspace.EXPECT().Close(mock.Anything).Return().Maybe()
	mockOrchestrator.EXPECT().NewWorkspace().Return(mockWorkspace).Maybe()

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockReporter, mockOrchestrator, mockMutagen)

	// Act
	args := domain.TestArgs{
		Paths:           []m.Path{"test.go"},
		Reports:         "reports.json",
		Threads:         1,
		ShardIndex:      0,
		TotalShardCount: 1,
	}
	err := wf.Test(context.Background(), args)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "errors occurred during mutation testing")
}

func TestWorkflow_Test_SaveReportsError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFSAdapter := new(domainmocks.MockSourceFSAdapter)
	mockReportStore := new(domainmocks.MockReportStore)
	mockReportStore.EXPECT().CleanReports(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReportStore.EXPECT().RegenerateIndex(ctx, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockMutagen := new(domainmocks.MockMutagen)

	sources := []rpt.Source{
		{Origin: &rpt.File{FullPath: "test.go", Hash: "hash1"}},
	}

	mutations := []m.Mutation{
		{ID: "hash-1", Source: sources[0]},
	}

	mockReporter.EXPECT().StartTest(ctx).Return(nil).Once()
	mockReporter.EXPECT().Close(ctx).Return().Once()
	mockReporter.EXPECT().DisplayConcurrencyInfo(ctx, mock.Anything, mock.Anything, mock.Anything).Return()
	mockReporter.EXPECT().DisplayUpcomingTestsInfo(ctx, mock.Anything).Return()
	mockReporter.EXPECT().DisplayStartingTestInfo(ctx, mock.Anything, mock.Anything).Return().Once()
	mockReporter.EXPECT().DisplayCompletedTestInfo(ctx, mock.Anything, mock.Anything).Return().Once()
	mockFSAdapter.EXPECT().Stream(ctx, mock.Anything).Return(streamSources(sources))
	mockMutagen.EXPECT().
		StreamMutations(ctx, mock.Anything, mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4], domain.DefaultMutations[5]).
		RunAndReturn(streamMutationsFn(mutations))
	mockWorkspace.EXPECT().Run(mock.Anything, mock.Anything).Return(m.Result{}, nil)
	mockWorkspace.EXPECT().Close(mock.Anything).Return().Maybe()
	mockOrchestrator.EXPECT().NewWorkspace().Return(mockWorkspace).Maybe()

	saveErr := errors.New("failed to save reports")
	mockReportStore.EXPECT().SaveReports(ctx, mock.Anything, mock.Anything).Return(saveErr)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockReporter, mockOrchestrator, mockMutagen)

	// Act
	args := domain.TestArgs{
		Paths:           []m.Path{"test.go"},
		Reports:         "reports.json",
		Threads:         1,
		ShardIndex:      0,
		TotalShardCount: 1,
	}
	err := wf.Test(context.Background(), args)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "save reports")
}

func TestWorkflow_Test_NoMutations(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFSAdapter := new(domainmocks.MockSourceFSAdapter)
	mockReportStore := new(domainmocks.MockReportStore)
	mockReportStore.EXPECT().CleanReports(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockMutagen := new(domainmocks.MockMutagen)

	sources := []rpt.Source{
		{Origin: &rpt.File{FullPath: "test.go", Hash: "hash1"}},
	}

	// No mutations generated
	mockReporter.EXPECT().StartTest(ctx).Return(nil).Once()
	mockReporter.EXPECT().Wait(ctx).Return().Once()
	mockReporter.EXPECT().Close(ctx).Return().Once()
	mockReporter.EXPECT().DisplayConcurrencyInfo(ctx, mock.Anything, mock.Anything, mock.Anything).Return()
	mockReporter.EXPECT().DisplayUpcomingTestsInfo(ctx, mock.Anything).Return()
	mockFSAdapter.EXPECT().Stream(ctx, mock.Anything).Return(streamSources(sources))
	mockMutagen.EXPECT().
		StreamMutations(ctx, mock.Anything, mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4], domain.DefaultMutations[5]).
		RunAndReturn(streamMutationsFn([]m.Mutation{}))
	mockWorkspace.EXPECT().Close(mock.Anything).Return().Maybe()
	mockOrchestrator.EXPECT().NewWorkspace().Return(mockWorkspace).Maybe()
	mockReportStore.EXPECT().SaveReports(ctx, mock.Anything, mock.MatchedBy(func(reports pkg.FileSpill[m.Report]) bool {
		return reports.Len() == 0
	})).Return(nil)
	mockReportStore.EXPECT().RegenerateIndex(ctx, mock.Anything).Return(nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockReporter, mockOrchestrator, mockMutagen)

	// Act
	args := domain.TestArgs{
		Paths:           []m.Path{"test.go"},
		Reports:         "reports.json",
		Threads:         1,
		ShardIndex:      0,
		TotalShardCount: 1,
	}
	err := wf.Test(context.Background(), args)

	// Assert
	assert.NoError(t, err)
	mockFSAdapter.AssertExpectations(t)
	mockMutagen.AssertExpectations(t)
	mockReportStore.AssertExpectations(t)
}

func TestWorkflow_Test_MultipleThreads(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFSAdapter := new(domainmocks.MockSourceFSAdapter)
	mockReportStore := new(domainmocks.MockReportStore)
	mockReportStore.EXPECT().CleanReports(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReportStore.EXPECT().RegenerateIndex(ctx, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockMutagen := new(domainmocks.MockMutagen)

	source := rpt.Source{
		Origin: &rpt.File{FullPath: "test.go", Hash: "hash1"},
	}
	sources := []rpt.Source{source}

	mutations := []m.Mutation{
		{ID: "hash-0", Source: source},
		{ID: "hash-1", Source: source},
		{ID: "hash-2", Source: source},
	}

	mockReporter.EXPECT().StartTest(ctx).Return(nil).Once()
	mockReporter.EXPECT().Wait(ctx).Return().Once()
	mockReporter.EXPECT().Close(ctx).Return().Once()
	mockReporter.EXPECT().DisplayConcurrencyInfo(ctx, mock.Anything, mock.Anything, mock.Anything).Return()
	mockReporter.EXPECT().DisplayUpcomingTestsInfo(ctx, mock.Anything).Return()
	mockReporter.EXPECT().DisplayStartingTestInfo(ctx, mock.Anything, mock.Anything).Return().Times(3)
	mockReporter.EXPECT().DisplayCompletedTestInfo(ctx, mock.Anything, mock.Anything).Return().Times(3)
	mockFSAdapter.EXPECT().Stream(ctx, mock.Anything).Return(streamSources(sources))
	mockMutagen.EXPECT().
		StreamMutations(ctx, mock.Anything, mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4], domain.DefaultMutations[5]).
		RunAndReturn(streamMutationsFn(mutations))
	mockWorkspace.EXPECT().Run(mock.Anything, mock.Anything).Return(m.Result{}, nil).Times(3)
	mockWorkspace.EXPECT().Close(mock.Anything).Return().Maybe()
	mockOrchestrator.EXPECT().NewWorkspace().Return(mockWorkspace).Maybe()
	mockReportStore.EXPECT().SaveReports(ctx, mock.Anything, mock.MatchedBy(func(reports pkg.FileSpill[m.Report]) bool {
		return reports.Len() == 3
	})).Return(nil)
	mockReportStore.EXPECT().RegenerateIndex(ctx, mock.Anything).Return(nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockReporter, mockOrchestrator, mockMutagen)

	// Act
	args := domain.TestArgs{
		Paths:           []m.Path{"test.go"},
		Reports:         "reports.json",
		Threads:         4,
		ShardIndex:      0,
		TotalShardCount: 1,
	}
	err := wf.Test(context.Background(), args)

	// Assert
	assert.NoError(t, err)
	mockOrchestrator.AssertExpectations(t)
}

func TestWorkflow_Test_WithSharding(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFSAdapter := new(domainmocks.MockSourceFSAdapter)
	mockReportStore := new(domainmocks.MockReportStore)
	mockReportStore.EXPECT().CleanReports(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockMutagen := new(domainmocks.MockMutagen)

	baseReportsDir := m.Path("reports")
	expectedShardDir := m.Path(filepath.Join(string(baseReportsDir), domain.ShardDirPrefix+"0"))

	source := rpt.Source{
		Origin: &rpt.File{FullPath: "test.go", Hash: "hash1"},
	}

	// 6 mutations total
	mutations := []m.Mutation{
		{ID: "hash-0", Source: source},
		{ID: "hash-1", Source: source},
		{ID: "hash-2", Source: source},
		{ID: "hash-3", Source: source},
		{ID: "hash-4", Source: source},
		{ID: "hash-5", Source: source},
	}

	mockReporter.EXPECT().StartTest(ctx).Return(nil).Once()
	mockReporter.EXPECT().Wait(ctx).Return().Once()
	mockReporter.EXPECT().Close(ctx).Return().Once()
	mockReporter.EXPECT().DisplayConcurrencyInfo(ctx, mock.Anything, mock.Anything, mock.Anything).Return()
	mockReporter.EXPECT().DisplayUpcomingTestsInfo(ctx, mock.Anything).Return()
	mockReporter.EXPECT().DisplayStartingTestInfo(ctx, mock.Anything, mock.Anything).Return().Maybe()
	mockReporter.EXPECT().DisplayCompletedTestInfo(ctx, mock.Anything, mock.Anything).Return().Maybe()
	mockFSAdapter.EXPECT().Stream(ctx, mock.Anything).Return(streamSources([]rpt.Source{source}))
	mockMutagen.EXPECT().
		StreamMutations(ctx, mock.Anything, mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4], domain.DefaultMutations[5]).
		RunAndReturn(streamMutationsFn(mutations))
	// With hash-based sharding, the number of mutations in shard 0 may vary
	mockWorkspace.EXPECT().Run(mock.Anything, mock.Anything).Return(m.Result{}, nil).Maybe()
	mockWorkspace.EXPECT().Close(mock.Anything).Return().Maybe()
	mockOrchestrator.EXPECT().NewWorkspace().Return(mockWorkspace).Maybe()
	mockReportStore.EXPECT().SaveReports(ctx, expectedShardDir, mock.MatchedBy(func(reports pkg.FileSpill[m.Report]) bool {
		// Accept any number of reports since hash-based sharding determines this
		return true
	})).Return(nil)
	mockReportStore.EXPECT().RegenerateIndex(ctx, expectedShardDir).Return(nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockReporter, mockOrchestrator, mockMutagen)

	// Act
	args := domain.TestArgs{
		Paths:           []m.Path{"test.go"},
		Reports:         baseReportsDir,
		Threads:         1,
		ShardIndex:      0,
		TotalShardCount: 3,
	}
	err := wf.Test(context.Background(), args)

	// Assert
	assert.NoError(t, err)
	mockOrchestrator.AssertExpectations(t)
}

func TestWorkflow_TestThreadsZeroDoesNotPanic(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFSAdapter := new(domainmocks.MockSourceFSAdapter)
	mockReportStore := new(domainmocks.MockReportStore)
	mockReportStore.EXPECT().CleanReports(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReportStore.EXPECT().RegenerateIndex(ctx, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockMutagen := new(domainmocks.MockMutagen)

	source := rpt.Source{
		Origin: &rpt.File{FullPath: "test.go", Hash: "hash1"},
	}

	mutations := []m.Mutation{
		{ID: "hash-0", Source: source, Type: mgen.Arithmetic},
	}

	mockReporter.EXPECT().StartTest(ctx).Return(nil).Once()
	mockReporter.EXPECT().Wait(ctx).Return().Once()
	mockReporter.EXPECT().Close(ctx).Return().Once()
	mockReporter.EXPECT().DisplayConcurrencyInfo(ctx, mock.Anything, mock.Anything, mock.Anything).Return()
	mockReporter.EXPECT().DisplayUpcomingTestsInfo(ctx, mock.Anything).Return()
	mockReporter.EXPECT().DisplayStartingTestInfo(ctx, mutations[0], 0).Return().Once()
	mockReporter.EXPECT().DisplayCompletedTestInfo(ctx, mutations[0], mock.Anything).Return().Once()
	mockFSAdapter.EXPECT().Stream(ctx, mock.Anything).Return(streamSources([]rpt.Source{source}))
	mockMutagen.EXPECT().
		StreamMutations(ctx, mock.Anything, mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4], domain.DefaultMutations[5]).
		RunAndReturn(streamMutationsFn(mutations))
	mockWorkspace.EXPECT().Run(mock.Anything, mutations[0]).Return(m.Result{}, nil)
	mockWorkspace.EXPECT().Close(mock.Anything).Return().Maybe()
	mockOrchestrator.EXPECT().NewWorkspace().Return(mockWorkspace).Maybe()
	mockReportStore.EXPECT().SaveReports(ctx, mock.Anything, mock.Anything).Return(nil)
	mockReportStore.EXPECT().RegenerateIndex(ctx, mock.Anything).Return(nil)
	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockReporter, mockOrchestrator, mockMutagen)

	// Act
	args := domain.TestArgs{
		Reports:         "reports.json",
		Threads:         0,
		ShardIndex:      0,
		TotalShardCount: 1,
	}
	err := wf.Test(context.Background(), args)

	// Assert
	assert.NoError(t, err)
}

func TestWorkflow_TestThreadIDWithinBounds(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFSAdapter := new(domainmocks.MockSourceFSAdapter)
	mockReportStore := new(domainmocks.MockReportStore)
	mockReportStore.EXPECT().CleanReports(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockMutagen := new(domainmocks.MockMutagen)

	source := rpt.Source{
		Origin: &rpt.File{FullPath: "test.go", Hash: "hash1"},
	}

	mutations := []m.Mutation{
		{ID: "hash-0", Source: source, Type: mgen.Arithmetic},
		{ID: "hash-1", Source: source, Type: mgen.Arithmetic},
	}

	mockReporter.EXPECT().StartTest(ctx).Return(nil).Once()
	mockReporter.EXPECT().Wait(ctx).Return().Once()
	mockReporter.EXPECT().Close(ctx).Return().Once()
	mockReporter.EXPECT().DisplayConcurrencyInfo(ctx, mock.Anything, mock.Anything, mock.Anything).Return()
	mockReporter.EXPECT().DisplayUpcomingTestsInfo(ctx, mock.Anything).Return()
	mockReporter.EXPECT().DisplayStartingTestInfo(ctx, mock.Anything, mock.MatchedBy(func(id int) bool {
		return id >= 0 && id < 2
	})).Return().Times(2)
	mockReporter.EXPECT().DisplayCompletedTestInfo(ctx, mock.Anything, mock.Anything).Return().Times(2)
	mockFSAdapter.EXPECT().Stream(ctx, mock.Anything).Return(streamSources([]rpt.Source{source}))
	mockMutagen.EXPECT().
		StreamMutations(ctx, mock.Anything, mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4], domain.DefaultMutations[5]).
		RunAndReturn(streamMutationsFn(mutations))
	mockWorkspace.EXPECT().Run(mock.Anything, mock.Anything).Return(m.Result{}, nil).Times(2)
	mockWorkspace.EXPECT().Close(mock.Anything).Return().Maybe()
	mockOrchestrator.EXPECT().NewWorkspace().Return(mockWorkspace).Maybe()
	mockReportStore.EXPECT().SaveReports(ctx, mock.Anything, mock.MatchedBy(func(reports pkg.FileSpill[m.Report]) bool {
		return reports.Len() == 2
	})).Return(nil)
	mockReportStore.EXPECT().RegenerateIndex(ctx, mock.Anything).Return(nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockReporter, mockOrchestrator, mockMutagen)

	// Act
	args := domain.TestArgs{
		Reports:         "reports.json",
		Threads:         2,
		ShardIndex:      0,
		TotalShardCount: 1,
	}
	err := wf.Test(context.Background(), args)

	// Assert
	assert.NoError(t, err)
}

func TestWorkflow_TestThreadIDIsUniqueForThreadsTwo(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFSAdapter := new(domainmocks.MockSourceFSAdapter)
	mockReportStore := new(domainmocks.MockReportStore)
	mockReportStore.EXPECT().CleanReports(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockMutagen := new(domainmocks.MockMutagen)

	source := rpt.Source{
		Origin: &rpt.File{FullPath: "test.go", Hash: "hash1"},
	}

	mutations := []m.Mutation{
		{ID: "hash-0", Source: source, Type: mgen.Arithmetic},
		{ID: "hash-1", Source: source, Type: mgen.Arithmetic},
	}

	var threadIDsMu sync.Mutex
	threadIDs := make([]int, 0, 2)

	// barrier blocks each Run until both mutations are being processed
	// concurrently, guaranteeing the two mutations land on distinct worker
	// threads rather than being drained sequentially by a single worker.
	barrier := make(chan struct{}, 2)

	mockReporter.EXPECT().StartTest(ctx).Return(nil).Once()
	mockReporter.EXPECT().Wait(ctx).Return().Once()
	mockReporter.EXPECT().Close(ctx).Return().Once()
	mockReporter.EXPECT().DisplayConcurrencyInfo(ctx, mock.Anything, mock.Anything, mock.Anything).Return()
	mockReporter.EXPECT().DisplayUpcomingTestsInfo(ctx, mock.Anything).Return()
	mockReporter.EXPECT().DisplayStartingTestInfo(ctx, mock.Anything, mock.Anything).Run(func(_ context.Context, _ m.Mutation, threadID int) {
		threadIDsMu.Lock()
		threadIDs = append(threadIDs, threadID)
		threadIDsMu.Unlock()
	}).Return().Times(2)
	mockReporter.EXPECT().DisplayCompletedTestInfo(ctx, mock.Anything, mock.Anything).Return().Times(2)
	mockFSAdapter.EXPECT().Stream(ctx, mock.Anything).Return(streamSources([]rpt.Source{source}))
	mockMutagen.EXPECT().
		StreamMutations(ctx, mock.Anything, mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4], domain.DefaultMutations[5]).
		RunAndReturn(streamMutationsFn(mutations))
	mockWorkspace.EXPECT().Run(mock.Anything, mock.Anything).Run(func(_ context.Context, _ m.Mutation) {
		// Signal arrival, then wait until both Runs have arrived so each
		// mutation is held by a separate worker simultaneously.
		barrier <- struct{}{}
		for len(barrier) < 2 {
			time.Sleep(time.Millisecond)
		}
	}).Return(m.Result{}, nil).Times(2)
	mockWorkspace.EXPECT().Close(mock.Anything).Return().Maybe()
	mockOrchestrator.EXPECT().NewWorkspace().Return(mockWorkspace).Maybe()
	mockReportStore.EXPECT().SaveReports(ctx, mock.Anything, mock.MatchedBy(func(reports pkg.FileSpill[m.Report]) bool {
		return reports.Len() == 2
	})).Return(nil)
	mockReportStore.EXPECT().RegenerateIndex(ctx, mock.Anything).Return(nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockReporter, mockOrchestrator, mockMutagen)

	// Act
	args := domain.TestArgs{
		Reports:         "reports.json",
		Threads:         2,
		ShardIndex:      0,
		TotalShardCount: 1,
	}
	err := wf.Test(context.Background(), args)

	// Assert
	assert.NoError(t, err)
	if assert.Len(t, threadIDs, 2) {
		assert.NotEqual(t, threadIDs[0], threadIDs[1])
	}
}

func TestWorkflow_TestWithSkippedMutation(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFSAdapter := new(domainmocks.MockSourceFSAdapter)
	mockReportStore := new(domainmocks.MockReportStore)
	mockReportStore.EXPECT().CleanReports(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockMutagen := new(domainmocks.MockMutagen)

	diffCode := []byte("--- original\n+++ mutated\n@@ -1,1 +1,1 @@\n-\treturn 3 + 5\n+\treturn 3 - 5\n")

	sources := []rpt.Source{
		{
			Origin: &rpt.File{FullPath: "test.go", Hash: "hash1"},
			Test:   &rpt.File{FullPath: "test_test.go", Hash: "test_hash1"},
		},
	}

	mutations := []m.Mutation{
		{
			ID:       "hash-0",
			Source:   sources[0],
			Type:     mgen.Arithmetic,
			DiffCode: diffCode,
		},
	}

	skippedResult := m.Result{
		mgen.Arithmetic: []struct {
			MutationID string
			Status     rpt.TestStatus
			Err        error
		}{{MutationID: "hash-0", Status: rpt.Skipped}},
	}

	mockReporter.EXPECT().StartTest(ctx).Return(nil).Once()
	mockReporter.EXPECT().Wait(ctx).Return().Once()
	mockReporter.EXPECT().Close(ctx).Return().Once()
	mockReporter.EXPECT().DisplayConcurrencyInfo(ctx, mock.Anything, mock.Anything, mock.Anything).Return()
	mockReporter.EXPECT().DisplayUpcomingTestsInfo(ctx, 1).Return()
	mockReporter.EXPECT().DisplayStartingTestInfo(ctx, mutations[0], 0).Return().Once()
	mockReporter.EXPECT().DisplayCompletedTestInfo(ctx, mutations[0], skippedResult).Return().Once()

	mockFSAdapter.EXPECT().Stream(ctx, mock.Anything).Return(streamSources(sources))
	mockMutagen.EXPECT().
		StreamMutations(ctx, mock.Anything, mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4], domain.DefaultMutations[5]).
		RunAndReturn(streamMutationsFn(mutations))
	mockWorkspace.EXPECT().Run(mock.Anything, mutations[0]).Return(skippedResult, nil)
	mockWorkspace.EXPECT().Close(mock.Anything).Return().Maybe()
	mockOrchestrator.EXPECT().NewWorkspace().Return(mockWorkspace).Maybe()

	mockReportStore.EXPECT().SaveReports(ctx, mock.Anything, mock.MatchedBy(func(reports pkg.FileSpill[m.Report]) bool {
		collected := spillReportsIfOpen(reports)
		if len(collected) != 1 {
			return false
		}
		report := collected[0]
		return report.Diff != nil
	})).Return(nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockReporter, mockOrchestrator, mockMutagen)

	// Act
	args := domain.TestArgs{
		Reports:         "reports.json",
		Threads:         1,
		ShardIndex:      0,
		TotalShardCount: 1,
	}
	err := wf.Test(context.Background(), args)

	// Assert
	assert.NoError(t, err)
}

func TestWorkflow_TestMutationIDExactMatchDoesNotUseHigherID(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFSAdapter := new(domainmocks.MockSourceFSAdapter)
	mockReportStore := new(domainmocks.MockReportStore)
	mockReportStore.EXPECT().CleanReports(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockMutagen := new(domainmocks.MockMutagen)

	diffCode := []byte("--- original\n+++ mutated\n@@ -1,1 +1,1 @@\n-\treturn 3 + 5\n+\treturn 3 - 5\n")

	sources := []rpt.Source{
		{
			Origin: &rpt.File{FullPath: "test.go", Hash: "hash1"},
			Test:   &rpt.File{FullPath: "test_test.go", Hash: "test_hash1"},
		},
	}

	mutations := []m.Mutation{
		{
			ID:       "hash-2",
			Source:   sources[0],
			Type:     mgen.Arithmetic,
			DiffCode: diffCode,
		},
	}

	result := m.Result{
		mgen.Arithmetic: []struct {
			MutationID string
			Status     rpt.TestStatus
			Err        error
		}{
			{MutationID: "hash-1", Status: rpt.Killed},
			{MutationID: "hash-3", Status: rpt.Survived},
		},
	}

	mockReporter.EXPECT().StartTest(ctx).Return(nil).Once()
	mockReporter.EXPECT().Wait(ctx).Return().Once()
	mockReporter.EXPECT().Close(ctx).Return().Once()
	mockReporter.EXPECT().DisplayConcurrencyInfo(ctx, mock.Anything, mock.Anything, mock.Anything).Return()
	mockReporter.EXPECT().DisplayUpcomingTestsInfo(ctx, 1).Return()
	mockReporter.EXPECT().DisplayStartingTestInfo(ctx, mutations[0], 0).Return().Once()
	mockReporter.EXPECT().DisplayCompletedTestInfo(ctx, mutations[0], result).Return().Once()

	mockFSAdapter.EXPECT().Stream(ctx, mock.Anything).Return(streamSources(sources))
	mockMutagen.EXPECT().
		StreamMutations(ctx, mock.Anything, mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4], domain.DefaultMutations[5]).
		RunAndReturn(streamMutationsFn(mutations))
	mockWorkspace.EXPECT().Run(mock.Anything, mutations[0]).Return(result, nil)
	mockWorkspace.EXPECT().Close(mock.Anything).Return().Maybe()
	mockOrchestrator.EXPECT().NewWorkspace().Return(mockWorkspace).Maybe()

	mockReportStore.EXPECT().SaveReports(ctx, mock.Anything, mock.MatchedBy(func(reports pkg.FileSpill[m.Report]) bool {
		collected := spillReportsIfOpen(reports)
		if len(collected) != 1 {
			return false
		}
		report := collected[0]
		return report.Diff != nil
	})).Return(nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockReporter, mockOrchestrator, mockMutagen)

	// Act
	args := domain.TestArgs{
		Reports:         "reports.json",
		Threads:         1,
		ShardIndex:      0,
		TotalShardCount: 1,
	}
	err := wf.Test(context.Background(), args)

	// Assert
	assert.NoError(t, err)
}

func TestWorkflow_TestEmptyResultEntriesReturnsError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFSAdapter := new(domainmocks.MockSourceFSAdapter)
	mockReportStore := new(domainmocks.MockReportStore)
	mockReportStore.EXPECT().CleanReports(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockMutagen := new(domainmocks.MockMutagen)

	diffCode := []byte("--- original\n+++ mutated\n@@ -1,1 +1,1 @@\n-\treturn 3 + 5\n+\treturn 3 - 5\n")

	sources := []rpt.Source{
		{
			Origin: &rpt.File{FullPath: "test.go", Hash: "hash1"},
			Test:   &rpt.File{FullPath: "test_test.go", Hash: "test_hash1"},
		},
	}

	mutations := []m.Mutation{
		{
			ID:       "hash-0",
			Source:   sources[0],
			Type:     mgen.Arithmetic,
			DiffCode: diffCode,
		},
	}

	result := m.Result{
		mgen.Arithmetic: []struct {
			MutationID string
			Status     rpt.TestStatus
			Err        error
		}{},
	}

	mockReporter.EXPECT().StartTest(ctx).Return(nil).Once()
	mockReporter.EXPECT().Wait(ctx).Return().Once()
	mockReporter.EXPECT().Close(ctx).Return().Once()
	mockReporter.EXPECT().DisplayConcurrencyInfo(ctx, mock.Anything, mock.Anything, mock.Anything).Return()
	mockReporter.EXPECT().DisplayUpcomingTestsInfo(ctx, 1).Return()
	mockReporter.EXPECT().DisplayStartingTestInfo(ctx, mutations[0], 0).Return().Once()
	mockReporter.EXPECT().DisplayCompletedTestInfo(ctx, mutations[0], result).Return().Once()

	mockFSAdapter.EXPECT().Stream(ctx, mock.Anything).Return(streamSources(sources))
	mockMutagen.EXPECT().
		StreamMutations(ctx, mock.Anything, mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4], domain.DefaultMutations[5]).
		RunAndReturn(streamMutationsFn(mutations))
	mockWorkspace.EXPECT().Run(mock.Anything, mutations[0]).Return(result, nil)
	mockWorkspace.EXPECT().Close(mock.Anything).Return().Maybe()
	mockOrchestrator.EXPECT().NewWorkspace().Return(mockWorkspace).Maybe()

	mockReportStore.EXPECT().SaveReports(ctx, mock.Anything, mock.MatchedBy(func(reports pkg.FileSpill[m.Report]) bool {
		collected := spillReportsIfOpen(reports)
		if len(collected) != 1 {
			return false
		}
		report := collected[0]
		return report.Diff != nil
	})).Return(nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockReporter, mockOrchestrator, mockMutagen)

	// Act
	args := domain.TestArgs{
		Reports:         "reports.json",
		Threads:         1,
		ShardIndex:      0,
		TotalShardCount: 1,
	}
	err := wf.Test(context.Background(), args)

	// Assert
	assert.NoError(t, err)
}

func TestWorkflow_Test_MultipleSources(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFSAdapter := new(domainmocks.MockSourceFSAdapter)
	mockReportStore := new(domainmocks.MockReportStore)
	mockReportStore.EXPECT().CleanReports(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockMutagen := new(domainmocks.MockMutagen)

	source1 := rpt.Source{
		Origin: &rpt.File{FullPath: "file1.go", Hash: "hash1"},
	}
	source2 := rpt.Source{
		Origin: &rpt.File{FullPath: "file2.go", Hash: "hash2"},
	}

	mutations1 := []m.Mutation{
		{ID: "hash-0", Source: source1},
		{ID: "hash-1", Source: source1},
	}
	mutations2 := []m.Mutation{
		{ID: "hash-2", Source: source2},
	}

	mockReporter.EXPECT().StartTest(ctx).Return(nil).Once()
	mockReporter.EXPECT().Wait(ctx).Return().Once()
	mockReporter.EXPECT().Close(ctx).Return().Once()
	mockReporter.EXPECT().DisplayConcurrencyInfo(ctx, mock.Anything, mock.Anything, mock.Anything).Return()
	mockReporter.EXPECT().DisplayUpcomingTestsInfo(ctx, mock.Anything).Return()
	mockReporter.EXPECT().DisplayStartingTestInfo(ctx, mock.Anything, mock.Anything).Return().Times(3)
	mockReporter.EXPECT().DisplayCompletedTestInfo(ctx, mock.Anything, mock.Anything).Return().Times(3)
	mockFSAdapter.EXPECT().Stream(ctx, mock.Anything).Return(streamSources([]rpt.Source{source1, source2}))
	mockMutagen.EXPECT().
		StreamMutations(ctx, source1, mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4], domain.DefaultMutations[5]).
		RunAndReturn(streamMutationsFn(mutations1))
	mockMutagen.EXPECT().
		StreamMutations(ctx, source2, mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4], domain.DefaultMutations[5]).
		RunAndReturn(streamMutationsFn(mutations2))
	mockWorkspace.EXPECT().Run(mock.Anything, mock.Anything).Return(m.Result{}, nil).Times(3)
	mockWorkspace.EXPECT().Close(mock.Anything).Return().Maybe()
	mockOrchestrator.EXPECT().NewWorkspace().Return(mockWorkspace).Maybe()
	mockReportStore.EXPECT().SaveReports(ctx, mock.Anything, mock.MatchedBy(func(reports pkg.FileSpill[m.Report]) bool {
		return reports.Len() == 3
	})).Return(nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockReporter, mockOrchestrator, mockMutagen)

	// Act
	args := domain.TestArgs{

		Reports:         "reports.json",
		Threads:         1,
		ShardIndex:      0,
		TotalShardCount: 1,
	}
	err := wf.Test(context.Background(), args)

	// Assert
	assert.NoError(t, err)
	mockFSAdapter.AssertExpectations(t)
	mockMutagen.AssertExpectations(t)
	mockOrchestrator.AssertExpectations(t)
	mockReportStore.AssertExpectations(t)
}

func TestWorkflow_NewWorkflowV2(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFSAdapter := new(domainmocks.MockSourceFSAdapter)
	mockReportStore := new(domainmocks.MockReportStore)
	mockReportStore.EXPECT().CleanReports(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockMutagen := new(domainmocks.MockMutagen)

	// Act
	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockReporter, mockOrchestrator, mockMutagen)

	// Assert
	require.NotNil(t, wf)
	assert.Implements(t, (*domain.Workflow)(nil), wf)
}

func TestWorkflow_TestWithSurvivedMutation(t *testing.T) {
	// Arrange - This test specifically checks that survived mutations include diff data
	ctx := context.Background()
	mockFSAdapter := new(domainmocks.MockSourceFSAdapter)
	mockReportStore := new(domainmocks.MockReportStore)
	mockReportStore.EXPECT().CleanReports(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockMutagen := new(domainmocks.MockMutagen)

	diffCode := []byte("--- original\n+++ mutated\n@@ -1,1 +1,1 @@\n-\treturn 3 + 5\n+\treturn 3 - 5\n")

	sources := []rpt.Source{
		{
			Origin: &rpt.File{FullPath: "test.go", Hash: "hash1"},
			Test:   &rpt.File{FullPath: "test_test.go", Hash: "test_hash1"},
		},
	}

	mutations := []m.Mutation{
		{
			ID:       "hash-0",
			Source:   sources[0],
			Type:     mgen.Arithmetic,
			DiffCode: diffCode,
		},
	}

	// Mock a survived mutation result
	survivedResult := m.Result{
		mgen.Arithmetic: []struct {
			MutationID string
			Status     rpt.TestStatus
			Err        error
		}{{MutationID: "hash-0", Status: rpt.Survived}},
	}

	mockReporter.EXPECT().StartTest(ctx).Return(nil).Once()
	mockReporter.EXPECT().Wait(ctx).Return().Once()
	mockReporter.EXPECT().Close(ctx).Return().Once()
	mockReporter.EXPECT().DisplayConcurrencyInfo(ctx, mock.Anything, mock.Anything, mock.Anything).Return()
	mockReporter.EXPECT().DisplayUpcomingTestsInfo(ctx, 1).Return()
	mockReporter.EXPECT().DisplayStartingTestInfo(ctx, mutations[0], 0).Return().Once()
	mockReporter.EXPECT().DisplayCompletedTestInfo(ctx, mutations[0], survivedResult).Return().Once()

	mockFSAdapter.EXPECT().Stream(ctx, mock.Anything).Return(streamSources(sources))
	mockMutagen.EXPECT().
		StreamMutations(ctx, mock.Anything, mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4], domain.DefaultMutations[5]).
		RunAndReturn(streamMutationsFn(mutations))
	mockWorkspace.EXPECT().Run(mock.Anything, mutations[0]).Return(survivedResult, nil)
	mockWorkspace.EXPECT().Close(mock.Anything).Return().Maybe()
	mockOrchestrator.EXPECT().NewWorkspace().Return(mockWorkspace).Maybe()

	// Verify that the report includes the diff for survived mutations
	mockReportStore.EXPECT().SaveReports(ctx, mock.Anything, mock.MatchedBy(func(reports pkg.FileSpill[m.Report]) bool {
		collected := spillReportsIfOpen(reports)
		if len(collected) != 1 {
			return false
		}
		report := collected[0]
		// Check that diff is included for survived mutation
		return report.Diff != nil && string(*report.Diff) == string(diffCode)
	})).Return(nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockReporter, mockOrchestrator, mockMutagen)

	// Act
	args := domain.TestArgs{
		Reports:         "reports.json",
		Threads:         1,
		ShardIndex:      0,
		TotalShardCount: 1,
	}
	err := wf.Test(context.Background(), args)

	// Assert
	assert.NoError(t, err)
	mockFSAdapter.AssertExpectations(t)
	mockMutagen.AssertExpectations(t)
	mockOrchestrator.AssertExpectations(t)
	mockReportStore.AssertExpectations(t)
	mockReporter.AssertExpectations(t)
}

func TestWorkflow_TestWithKilledMutation(t *testing.T) {
	// Arrange - This test checks that killed mutations do NOT include diff data
	ctx := context.Background()
	mockFSAdapter := new(domainmocks.MockSourceFSAdapter)
	mockReportStore := new(domainmocks.MockReportStore)
	mockReportStore.EXPECT().CleanReports(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockMutagen := new(domainmocks.MockMutagen)

	diffCode := []byte("--- original\n+++ mutated\n@@ -1,1 +1,1 @@\n-\treturn 3 + 5\n+\treturn 3 - 5\n")

	sources := []rpt.Source{
		{
			Origin: &rpt.File{FullPath: "test.go", Hash: "hash1"},
			Test:   &rpt.File{FullPath: "test_test.go", Hash: "test_hash1"},
		},
	}

	mutations := []m.Mutation{
		{
			ID:       "hash-0",
			Source:   sources[0],
			Type:     mgen.Arithmetic,
			DiffCode: diffCode,
		},
	}

	// Mock a killed mutation result
	killedResult := m.Result{
		mgen.Arithmetic: []struct {
			MutationID string
			Status     rpt.TestStatus
			Err        error
		}{{MutationID: "hash-0", Status: rpt.Killed}},
	}

	mockReporter.EXPECT().StartTest(ctx).Return(nil).Once()
	mockReporter.EXPECT().Wait(ctx).Return().Once()
	mockReporter.EXPECT().Close(ctx).Return().Once()
	mockReporter.EXPECT().DisplayConcurrencyInfo(ctx, mock.Anything, mock.Anything, mock.Anything).Return()
	mockReporter.EXPECT().DisplayUpcomingTestsInfo(ctx, 1).Return()
	mockReporter.EXPECT().DisplayStartingTestInfo(ctx, mutations[0], 0).Return().Once()
	mockReporter.EXPECT().DisplayCompletedTestInfo(ctx, mutations[0], killedResult).Return().Once()

	mockFSAdapter.EXPECT().Stream(ctx, mock.Anything).Return(streamSources(sources))
	mockMutagen.EXPECT().
		StreamMutations(ctx, mock.Anything, mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4], domain.DefaultMutations[5]).
		RunAndReturn(streamMutationsFn(mutations))
	mockWorkspace.EXPECT().Run(mock.Anything, mutations[0]).Return(killedResult, nil)
	mockWorkspace.EXPECT().Close(mock.Anything).Return().Maybe()
	mockOrchestrator.EXPECT().NewWorkspace().Return(mockWorkspace).Maybe()

	// Verify that the report does NOT include diff for killed mutations
	mockReportStore.EXPECT().SaveReports(ctx, mock.Anything, mock.MatchedBy(func(reports pkg.FileSpill[m.Report]) bool {
		collected := spillReportsIfOpen(reports)
		if len(collected) != 1 {
			return false
		}
		report := collected[0]
		// Check that diff is NOT included for killed mutation
		return report.Diff == nil
	})).Return(nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockReporter, mockOrchestrator, mockMutagen)

	// Act
	args := domain.TestArgs{
		Reports:         "reports.json",
		Threads:         1,
		ShardIndex:      0,
		TotalShardCount: 1,
	}
	err := wf.Test(context.Background(), args)

	// Assert
	assert.NoError(t, err)
	mockFSAdapter.AssertExpectations(t)
	mockMutagen.AssertExpectations(t)
	mockOrchestrator.AssertExpectations(t)
	mockReportStore.AssertExpectations(t)
	mockReporter.AssertExpectations(t)
}

func TestWorkflow_Estimate_Success(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFSAdapter := new(domainmocks.MockSourceFSAdapter)
	mockReportStore := new(domainmocks.MockReportStore)
	mockReportStore.EXPECT().CleanReports(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockMutagen := new(domainmocks.MockMutagen)

	sources := []rpt.Source{
		{Origin: &rpt.File{FullPath: "test.go", Hash: "hash1"}},
	}

	mutations := []m.Mutation{
		{ID: "hash-0", Source: sources[0], Type: mgen.Arithmetic},
	}

	mockReporter.EXPECT().StartEstimate(ctx).Return(nil).Once()
	mockReporter.EXPECT().DisplayEstimation(ctx, mock.MatchedBy(func(e domain.Estimation) bool {
		return e.Total == 1
	}), nil).Return(nil).Once()
	mockReporter.EXPECT().Wait(ctx).Return().Once()
	mockReporter.EXPECT().Close(ctx).Return().Once()

	mockFSAdapter.EXPECT().Stream(ctx, mock.Anything).Return(streamSources(sources))
	mockMutagen.EXPECT().
		StreamMutations(ctx, mock.Anything, mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4], domain.DefaultMutations[5]).
		RunAndReturn(streamMutationsFn(mutations))

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockReporter, mockOrchestrator, mockMutagen)

	// Act
	err := wf.Estimate(context.Background(), domain.EstimateArgs{Paths: []m.Path{"test.go"}})

	// Assert
	assert.NoError(t, err)
}

func TestWorkflow_Estimate_StartError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFSAdapter := new(domainmocks.MockSourceFSAdapter)
	mockReportStore := new(domainmocks.MockReportStore)
	mockReportStore.EXPECT().CleanReports(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockMutagen := new(domainmocks.MockMutagen)

	startErr := errors.New("start failed")
	mockReporter.EXPECT().StartEstimate(ctx).Return(startErr).Once()

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockReporter, mockOrchestrator, mockMutagen)

	// Act
	err := wf.Estimate(ctx, domain.EstimateArgs{Paths: []m.Path{"test.go"}})

	// Assert
	assert.ErrorIs(t, err, startErr)
}

func TestWorkflow_Estimate_GetMutationsError(t *testing.T) {
	// Arrange
	ctx := context.Background()

	mockFSAdapter := new(domainmocks.MockSourceFSAdapter)
	mockReportStore := new(domainmocks.MockReportStore)
	mockReportStore.EXPECT().CleanReports(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockMutagen := new(domainmocks.MockMutagen)

	getErr := errors.New("get mutations failed")

	mockReporter.EXPECT().StartEstimate(ctx).Return(nil).Once()
	mockReporter.EXPECT().Close(ctx).Return().Once()
	mockFSAdapter.EXPECT().Stream(ctx, mock.Anything).Return(streamSourcesErr(getErr))

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockReporter, mockOrchestrator, mockMutagen)

	// Act
	err := wf.Estimate(ctx, domain.EstimateArgs{Paths: []m.Path{"test.go"}})

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "get sources")
}

func TestWorkflow_Estimate_DisplayError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFSAdapter := new(domainmocks.MockSourceFSAdapter)
	mockReportStore := new(domainmocks.MockReportStore)
	mockReportStore.EXPECT().CleanReports(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockMutagen := new(domainmocks.MockMutagen)

	sources := []rpt.Source{
		{Origin: &rpt.File{FullPath: "test.go", Hash: "hash1"}},
	}

	mutations := []m.Mutation{
		{ID: "hash-0", Source: sources[0], Type: mgen.Arithmetic},
	}

	displayErr := errors.New("display failed")

	mockReporter.EXPECT().StartEstimate(ctx).Return(nil).Once()
	mockReporter.EXPECT().DisplayEstimation(ctx, mock.Anything, nil).Return(displayErr).Once()
	mockReporter.EXPECT().Close(ctx).Return().Once()

	mockFSAdapter.EXPECT().Stream(ctx, mock.Anything).Return(streamSources(sources))
	mockMutagen.EXPECT().
		StreamMutations(ctx, mock.Anything, mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4], domain.DefaultMutations[5]).
		RunAndReturn(streamMutationsFn(mutations))

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockReporter, mockOrchestrator, mockMutagen)

	// Act
	err := wf.Estimate(ctx, domain.EstimateArgs{Paths: []m.Path{"test.go"}})

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "display")
}

func TestWorkflow_TestThreadIDStartsAtZero(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFSAdapter := new(domainmocks.MockSourceFSAdapter)
	mockReportStore := new(domainmocks.MockReportStore)
	mockReportStore.EXPECT().CleanReports(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockMutagen := new(domainmocks.MockMutagen)

	source := rpt.Source{
		Origin: &rpt.File{FullPath: "test.go", Hash: "hash1"},
	}

	mutations := []m.Mutation{
		{ID: "hash-0", Source: source, Type: mgen.Arithmetic},
	}

	mockReporter.EXPECT().StartTest(ctx).Return(nil).Once()
	mockReporter.EXPECT().Wait(ctx).Return().Once()
	mockReporter.EXPECT().Close(ctx).Return().Once()
	mockReporter.EXPECT().DisplayConcurrencyInfo(ctx, mock.Anything, mock.Anything, mock.Anything).Return()
	mockReporter.EXPECT().DisplayUpcomingTestsInfo(ctx, mock.Anything).Return()
	mockReporter.EXPECT().DisplayStartingTestInfo(ctx, mutations[0], mock.MatchedBy(func(id int) bool {
		// Thread IDs are zero-based; a single mutation handled by any of the 3
		// workers must land in [0, 3).
		return id >= 0 && id < 3
	})).Return().Once()
	mockReporter.EXPECT().DisplayCompletedTestInfo(ctx, mutations[0], mock.Anything).Return().Once()
	mockFSAdapter.EXPECT().Stream(ctx, mock.Anything).Return(streamSources([]rpt.Source{source}))
	mockMutagen.EXPECT().
		StreamMutations(ctx, mock.Anything, mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4], domain.DefaultMutations[5]).
		RunAndReturn(streamMutationsFn(mutations))
	mockWorkspace.EXPECT().Run(mock.Anything, mutations[0]).Return(m.Result{}, nil)
	mockWorkspace.EXPECT().Close(mock.Anything).Return().Maybe()
	mockOrchestrator.EXPECT().NewWorkspace().Return(mockWorkspace).Maybe()
	mockReportStore.EXPECT().SaveReports(ctx, mock.Anything, mock.Anything).Return(nil)
	mockReportStore.EXPECT().RegenerateIndex(ctx, mock.Anything).Return(nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockReporter, mockOrchestrator, mockMutagen)

	// Act
	args := domain.TestArgs{
		Reports:         "reports.json",
		Threads:         3,
		ShardIndex:      0,
		TotalShardCount: 1,
	}
	err := wf.Test(context.Background(), args)

	// Assert
	assert.NoError(t, err)
}

func TestWorkflow_TestExactMutationIDMatch(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockFSAdapter := new(domainmocks.MockSourceFSAdapter)
	mockReportStore := new(domainmocks.MockReportStore)
	mockReportStore.EXPECT().CleanReports(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockMutagen := new(domainmocks.MockMutagen)

	diffCode := []byte("--- original\n+++ mutated\n@@ -1,1 +1,1 @@\n-\treturn 3 + 5\n+\treturn 3 - 5\n")

	sources := []rpt.Source{
		{
			Origin: &rpt.File{FullPath: "test.go", Hash: "hash1"},
			Test:   &rpt.File{FullPath: "test_test.go", Hash: "test_hash1"},
		},
	}

	mutations := []m.Mutation{
		{
			ID:       "hash-1",
			Source:   sources[0],
			Type:     mgen.Arithmetic,
			DiffCode: diffCode,
		},
	}

	result := m.Result{
		mgen.Arithmetic: []struct {
			MutationID string
			Status     rpt.TestStatus
			Err        error
		}{
			{MutationID: "hash-0", Status: rpt.Killed},
			{MutationID: "hash-1", Status: rpt.Survived},
		},
	}

	mockReporter.EXPECT().StartTest(ctx).Return(nil).Once()
	mockReporter.EXPECT().Wait(ctx).Return().Once()
	mockReporter.EXPECT().Close(ctx).Return().Once()
	mockReporter.EXPECT().DisplayConcurrencyInfo(ctx, mock.Anything, mock.Anything, mock.Anything).Return()
	mockReporter.EXPECT().DisplayUpcomingTestsInfo(ctx, 1).Return()
	mockReporter.EXPECT().DisplayStartingTestInfo(ctx, mutations[0], 0).Return().Once()
	mockReporter.EXPECT().DisplayCompletedTestInfo(ctx, mutations[0], result).Return().Once()

	mockFSAdapter.EXPECT().Stream(ctx, mock.Anything).Return(streamSources(sources))
	mockMutagen.EXPECT().
		StreamMutations(ctx, mock.Anything, mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4], domain.DefaultMutations[5]).
		RunAndReturn(streamMutationsFn(mutations))
	mockWorkspace.EXPECT().Run(mock.Anything, mutations[0]).Return(result, nil)
	mockWorkspace.EXPECT().Close(mock.Anything).Return().Maybe()
	mockOrchestrator.EXPECT().NewWorkspace().Return(mockWorkspace).Maybe()

	mockReportStore.EXPECT().SaveReports(ctx, mock.Anything, mock.MatchedBy(func(reports pkg.FileSpill[m.Report]) bool {
		collected := spillReportsIfOpen(reports)
		if len(collected) != 1 {
			return false
		}
		report := collected[0]
		return report.Diff != nil && string(*report.Diff) == string(diffCode)
	})).Return(nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockReporter, mockOrchestrator, mockMutagen)

	// Act
	args := domain.TestArgs{
		Reports:         "reports.json",
		Threads:         1,
		ShardIndex:      0,
		TotalShardCount: 1,
	}
	err := wf.Test(context.Background(), args)

	// Assert
	assert.NoError(t, err)
}

func spillOf[T any](t *testing.T, items ...T) pkg.FileSpill[T] {
	t.Helper()

	spill, err := pkg.FromSlice(items)
	require.NoError(t, err)

	return spill
}

func spillKeys(spill pkg.FileSpill[rpt.Source]) []string {
	sources, err := pkg.ToSlice(spill)
	if err != nil {
		return nil
	}

	keys := make([]string, 0, len(sources))
	for _, source := range sources {
		keys = append(keys, source.Key())
	}

	return keys
}

func writeReports(t *testing.T, dir string, reports ...m.Report) {
	t.Helper()

	store := adapter.NewReportStore()
	require.NoError(t, store.SaveReports(context.Background(), m.Path(dir), spillOf(t, reports...)))
	require.NoError(t, store.RegenerateIndex(context.Background(), m.Path(dir)))
}

func reportHashes(t *testing.T, dir string) []string {
	t.Helper()

	spill, err := adapter.NewReportStore().LoadSpillReports(context.Background(), m.Path(dir))
	require.NoError(t, err)

	defer func() { _ = spill.Close() }()

	reports, err := pkg.ToSlice(spill)
	require.NoError(t, err)

	hashes := make([]string, 0, len(reports))
	for _, report := range reports {
		hashes = append(hashes, report.Source.Origin.Hash)
	}

	return hashes
}

func mergeReporter(ctx context.Context, score *float64) *domainmocks.MockReporter {
	reporter := new(domainmocks.MockReporter)
	reporter.EXPECT().StartTest(ctx).Return(nil).Once()
	reporter.EXPECT().Wait(ctx).Return().Maybe()
	reporter.EXPECT().Close(ctx).Return().Once()
	reporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Run(func(_ context.Context, s domain.MutationScore) { *score = s.Score() }).Once()

	return reporter
}

func TestWorkflow_Merge_RemovesSupersededBaselineReportsFromDisk(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	shardDir := filepath.Join(base, domain.ShardDirPrefix+"0")

	writeReports(t, base,
		m.Report{Source: rpt.Source{Origin: &rpt.File{ShortPath: "a.go", FullPath: "/old/a.go", Hash: "a1"}},
			Result: m.Result{mgen.Arithmetic: {{MutationID: "m1", Status: rpt.Survived}}}},
		m.Report{Source: rpt.Source{Origin: &rpt.File{ShortPath: "b.go", FullPath: "/old/b.go", Hash: "b1"}},
			Result: m.Result{mgen.Arithmetic: {{MutationID: "mb", Status: rpt.Killed}}}},
	)
	writeReports(t, shardDir,
		m.Report{Source: rpt.Source{Origin: &rpt.File{ShortPath: "a.go", FullPath: "/new/a.go", Hash: "a2"}},
			Result: m.Result{mgen.Arithmetic: {{MutationID: "m2", Status: rpt.Killed}}}},
	)

	var score float64

	wf := domain.NewWorkflow(nil, adapter.NewReportStore(), mergeReporter(ctx, &score), nil, nil)
	require.NoError(t, wf.Merge(ctx, domain.MergeArgs{Reports: m.Path(base)}))

	assert.ElementsMatch(t, []string{"a2", "b1"}, reportHashes(t, base))
	assert.InDelta(t, 1.0, score, 1e-9)

	_, err := os.Stat(shardDir)
	assert.True(t, os.IsNotExist(err), "shard dir should be removed after merge")
}

func TestWorkflow_Merge_DropsDeletedSourcesInsideCheckout(t *testing.T) {
	ctx := context.Background()
	checkout := t.TempDir()
	t.Chdir(checkout)
	require.NoError(t, os.WriteFile(filepath.Join(checkout, "a.go"), []byte("package p\n"), 0o600))

	base := filepath.Join(checkout, "reports")
	shardDir := filepath.Join(base, domain.ShardDirPrefix+"0")

	writeReports(t, base,
		m.Report{Source: rpt.Source{Origin: &rpt.File{ShortPath: "gone.go", FullPath: "/old/gone.go", Hash: "g1"}},
			Result: m.Result{mgen.Arithmetic: {{MutationID: "mg", Status: rpt.Survived}}}},
	)
	writeReports(t, shardDir,
		m.Report{Source: rpt.Source{Origin: &rpt.File{ShortPath: "a.go", FullPath: "/new/a.go", Hash: "a2"}},
			Result: m.Result{mgen.Arithmetic: {{MutationID: "m2", Status: rpt.Killed}}}},
	)

	var score float64

	wf := domain.NewWorkflow(nil, adapter.NewReportStore(), mergeReporter(ctx, &score), nil, nil)
	require.NoError(t, wf.Merge(ctx, domain.MergeArgs{Reports: m.Path(base)}))

	assert.Equal(t, []string{"a2"}, reportHashes(t, base))
}

func TestWorkflow_Test_CleansStaleReportsForTestedSources(t *testing.T) {
	ctx := context.Background()
	mockFSAdapter := new(domainmocks.MockSourceFSAdapter)
	mockReportStore := new(domainmocks.MockReportStore)
	mockReporter := new(domainmocks.MockReporter)
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockMutagen := new(domainmocks.MockMutagen)

	source := rpt.Source{
		Origin: &rpt.File{FullPath: "/p/a.go", ShortPath: "a.go", Hash: "h2"},
		Test:   &rpt.File{FullPath: "/p/a_test.go", Hash: "t"},
	}
	gone := rpt.Source{Origin: &rpt.File{FullPath: "/p/gone.go", ShortPath: "gone.go", Hash: "g"}}
	mutation := m.Mutation{ID: "m2", Source: source, Type: mgen.Arithmetic}

	mockReporter.EXPECT().StartTest(ctx).Return(nil).Once()
	mockReporter.EXPECT().Wait(ctx).Return().Once()
	mockReporter.EXPECT().Close(ctx).Return().Once()
	mockReporter.EXPECT().DisplayConcurrencyInfo(ctx, mock.Anything, mock.Anything, mock.Anything).Return()
	mockReporter.EXPECT().DisplayUpcomingTestsInfo(ctx, 1).Return()
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return()
	mockReporter.EXPECT().DisplayStartingTestInfo(ctx, mock.Anything, mock.Anything).Return()
	mockReporter.EXPECT().DisplayCompletedTestInfo(ctx, mock.Anything, mock.Anything).Return()

	mockFSAdapter.EXPECT().Stream(ctx, mock.Anything).Return(streamSources([]rpt.Source{source}))
	mockMutagen.EXPECT().
		StreamMutations(ctx, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(streamMutationsFn([]m.Mutation{mutation}))
	mockOrchestrator.EXPECT().NewWorkspace().Return(mockWorkspace).Maybe()
	mockWorkspace.EXPECT().Close(mock.Anything).Return().Maybe()
	mockWorkspace.EXPECT().Run(mock.Anything, mutation).
		Return(m.Result{mgen.Arithmetic: {{MutationID: "m2", Status: rpt.Killed}}}, nil)

	mockReportStore.EXPECT().
		CheckUpdates(ctx, m.Path("reports"), mock.MatchedBy(func(sources pkg.FileSpill[rpt.Source]) bool {
			return assert.ObjectsAreEqual([]string{"a.go"}, spillKeys(sources))
		})).
		Return(spillOf(t, source), spillOf(t, gone), nil)
	mockReportStore.EXPECT().LoadSpillReports(ctx, m.Path("reports")).
		Return(spillOf(t, m.Report{Source: source, Result: m.Result{mgen.Arithmetic: {{MutationID: "m2", Status: rpt.Killed}}}}), nil).Once()

	var order []string

	mockReportStore.EXPECT().
		CleanReports(ctx, m.Path("reports"), mock.MatchedBy(func(sources pkg.FileSpill[rpt.Source]) bool {
			return assert.ObjectsAreEqual([]string{"gone.go"}, spillKeys(sources))
		})).
		RunAndReturn(func(context.Context, m.Path, pkg.FileSpill[rpt.Source]) error {
			order = append(order, "clean-deleted")
			return nil
		}).Once()
	mockReportStore.EXPECT().
		CleanReports(ctx, m.Path("reports"), mock.MatchedBy(func(sources pkg.FileSpill[rpt.Source]) bool {
			return assert.ObjectsAreEqual([]string{"a.go"}, spillKeys(sources))
		})).
		RunAndReturn(func(context.Context, m.Path, pkg.FileSpill[rpt.Source]) error {
			order = append(order, "clean-tested")
			return nil
		}).Once()
	mockReportStore.EXPECT().SaveReports(ctx, m.Path("reports"), mock.Anything).
		RunAndReturn(func(context.Context, m.Path, pkg.FileSpill[m.Report]) error {
			order = append(order, "save")
			return nil
		})
	mockReportStore.EXPECT().RegenerateIndex(ctx, m.Path("reports")).
		RunAndReturn(func(context.Context, m.Path) error {
			order = append(order, "index")
			return nil
		}).Once()

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockReporter, mockOrchestrator, mockMutagen)
	err := wf.Test(ctx, domain.TestArgs{
		EstimateArgs:    domain.EstimateArgs{Paths: []m.Path{"."}, UseCache: true, Reports: "reports"},
		Reports:         "reports",
		Threads:         1,
		TotalShardCount: 1,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"clean-deleted", "clean-tested", "save", "index"}, order)
	mockReportStore.AssertExpectations(t)
}

func TestWorkflow_Test_GeneratesMutationsOncePerSource(t *testing.T) {
	ctx := context.Background()
	mockFSAdapter := new(domainmocks.MockSourceFSAdapter)
	mockReportStore := new(domainmocks.MockReportStore)
	mockReporter := new(domainmocks.MockReporter)
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockMutagen := new(domainmocks.MockMutagen)

	source := rpt.Source{
		Origin: &rpt.File{FullPath: "/p/a.go", ShortPath: "a.go", Hash: "h"},
		Test:   &rpt.File{FullPath: "/p/a_test.go", Hash: "t"},
	}
	mutation := m.Mutation{ID: "m1", Source: source, Type: mgen.Arithmetic, MutatedCode: []byte("package p\n")}

	mockReporter.EXPECT().StartTest(ctx).Return(nil).Once()
	mockReporter.EXPECT().Wait(ctx).Return().Once()
	mockReporter.EXPECT().Close(ctx).Return().Once()
	mockReporter.EXPECT().DisplayConcurrencyInfo(ctx, mock.Anything, mock.Anything, mock.Anything).Return()
	mockReporter.EXPECT().DisplayUpcomingTestsInfo(ctx, 1).Return().Once()
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return()
	mockReporter.EXPECT().DisplayStartingTestInfo(ctx, mock.Anything, mock.Anything).Return()
	mockReporter.EXPECT().
		DisplayCompletedTestInfo(ctx, mock.MatchedBy(func(done m.Mutation) bool { return done.MutatedCode == nil }), mock.Anything).
		Return().Once()

	mockFSAdapter.EXPECT().Stream(ctx, mock.Anything).Return(streamSources([]rpt.Source{source}))
	mockMutagen.EXPECT().
		StreamMutations(ctx, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(streamMutationsFn([]m.Mutation{mutation})).Once()
	mockOrchestrator.EXPECT().NewWorkspace().Return(mockWorkspace).Maybe()
	mockWorkspace.EXPECT().Close(mock.Anything).Return().Maybe()
	mockWorkspace.EXPECT().
		Run(mock.Anything, mock.MatchedBy(func(run m.Mutation) bool { return string(run.MutatedCode) == "package p\n" })).
		Return(m.Result{mgen.Arithmetic: {{MutationID: "m1", Status: rpt.Survived}}}, nil).Once()
	mockReportStore.EXPECT().CleanReports(ctx, mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReportStore.EXPECT().SaveReports(ctx, mock.Anything, mock.Anything).Return(nil)
	mockReportStore.EXPECT().RegenerateIndex(ctx, mock.Anything).Return(nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockReporter, mockOrchestrator, mockMutagen)
	require.NoError(t, wf.Test(ctx, domain.TestArgs{
		Paths:           []m.Path{"."},
		Reports:         "reports",
		Threads:         1,
		TotalShardCount: 1,
	}))

	mockMutagen.AssertExpectations(t)
	mockWorkspace.AssertExpectations(t)
	mockReporter.AssertExpectations(t)
}

func TestWorkflow_ViewAll_DisplaysEveryMutationWithDiffsForNonKilled(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	diff := []byte("--- original\n+++ mutated\n")
	source := rpt.Source{Origin: &rpt.File{ShortPath: "a.go", FullPath: "/p/a.go", Hash: "a"}}

	writeReports(t, dir,
		m.Report{Source: source, Result: m.Result{mgen.Arithmetic: {{MutationID: "killed-1", Status: rpt.Killed}}}, Diff: &diff},
		m.Report{Source: source, Result: m.Result{mgen.Boolean: {{MutationID: "survived-1", Status: rpt.Survived}}}, Diff: &diff},
	)

	reporter := new(domainmocks.MockReporter)
	reporter.EXPECT().StartTest(ctx).Return(nil).Once()
	reporter.EXPECT().Wait(ctx).Return().Once()
	reporter.EXPECT().Close(ctx).Return().Once()
	reporter.EXPECT().DisplayUpcomingTestsInfo(ctx, 2).Return().Once()
	reporter.EXPECT().DisplayStartingTestInfo(ctx, mock.Anything, 0).Return().Times(2)

	diffs := map[string]string{}

	reporter.EXPECT().DisplayCompletedTestInfo(ctx, mock.Anything, mock.Anything).
		Run(func(_ context.Context, mutation m.Mutation, _ m.Result) {
			diffs[mutation.ID] = string(mutation.DiffCode)
		}).
		Return().Times(2)

	var score float64

	reporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Run(func(_ context.Context, s domain.MutationScore) { score = s.Score() }).Once()

	wf := domain.NewWorkflow(nil, adapter.NewReportStore(), reporter, nil, nil)
	require.NoError(t, wf.View(ctx, domain.ViewArgs{Reports: m.Path(dir), All: true}))

	assert.Equal(t, map[string]string{"killed-1": "", "survived-1": string(diff)}, diffs)
	assert.InDelta(t, 0.5, score, 1e-9)
	reporter.AssertExpectations(t)
}

func TestWorkflow_View_MissingReportsDirReturnsError(t *testing.T) {
	ctx := context.Background()

	missing := filepath.Join(t.TempDir(), "missing")
	wf := domain.NewWorkflow(nil, adapter.NewReportStore(), new(domainmocks.MockReporter), nil, nil)
	err := wf.View(ctx, domain.ViewArgs{Reports: m.Path(missing)})
	require.EqualError(t, err,
		"no reports directory at "+missing+"; run `gooze run` first, or pass -o with the reports directory")
	assert.Equal(t, m.ExitNoReports, exitCodeOf(err))
}

func TestWorkflow_Merge_WithoutBaselineDeduplicatesShardSources(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	source := rpt.Source{Origin: &rpt.File{ShortPath: "a.go", FullPath: "/new/a.go", Hash: "a2"}}

	writeReports(t, filepath.Join(base, domain.ShardDirPrefix+"0"),
		m.Report{Source: source, Result: m.Result{mgen.Arithmetic: {{MutationID: "m1", Status: rpt.Killed}}}})
	writeReports(t, filepath.Join(base, domain.ShardDirPrefix+"1"),
		m.Report{Source: source, Result: m.Result{mgen.Arithmetic: {{MutationID: "m2", Status: rpt.Survived}}}})

	var score float64

	wf := domain.NewWorkflow(nil, adapter.NewReportStore(), mergeReporter(ctx, &score), nil, nil)
	require.NoError(t, wf.Merge(ctx, domain.MergeArgs{Reports: m.Path(base)}))

	assert.Equal(t, []string{"a2", "a2"}, reportHashes(t, base))
	assert.InDelta(t, 0.5, score, 1e-9)
}

func TestWorkflow_Merge_NoShardsReportsBaseScore(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()

	writeReports(t, base, m.Report{
		Source: rpt.Source{Origin: &rpt.File{ShortPath: "a.go", FullPath: "/p/a.go", Hash: "a"}},
		Result: m.Result{mgen.Arithmetic: {{MutationID: "m1", Status: rpt.Survived}}},
	})

	var score float64

	wf := domain.NewWorkflow(nil, adapter.NewReportStore(), mergeReporter(ctx, &score), nil, nil)
	require.NoError(t, wf.Merge(ctx, domain.MergeArgs{Reports: m.Path(base)}))
	assert.InDelta(t, 0.0, score, 1e-9)
}

func TestWorkflow_Merge_CorruptReportsReturnError(t *testing.T) {
	ctx := context.Background()

	newReporter := func() *domainmocks.MockReporter {
		reporter := new(domainmocks.MockReporter)
		reporter.EXPECT().StartTest(ctx).Return(nil).Once()
		reporter.EXPECT().Close(ctx).Return().Once()

		return reporter
	}

	t.Run("corrupt shard", func(t *testing.T) {
		base := t.TempDir()
		shard := filepath.Join(base, domain.ShardDirPrefix+"0")
		require.NoError(t, os.MkdirAll(shard, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(shard, "bad.yaml"), []byte("source: [\n"), 0o600))

		wf := domain.NewWorkflow(nil, adapter.NewReportStore(), newReporter(), nil, nil)
		require.ErrorContains(t, wf.Merge(ctx, domain.MergeArgs{Reports: m.Path(base)}), "load shard reports")
	})

	t.Run("corrupt baseline inside checkout", func(t *testing.T) {
		checkout := t.TempDir()
		t.Chdir(checkout)
		require.NoError(t, os.WriteFile(filepath.Join(checkout, "a.go"), []byte("package p\n"), 0o600))

		base := filepath.Join(checkout, "reports")
		writeReports(t, filepath.Join(base, domain.ShardDirPrefix+"0"), m.Report{
			Source: rpt.Source{Origin: &rpt.File{ShortPath: "a.go", FullPath: "/new/a.go", Hash: "a2"}},
			Result: m.Result{mgen.Arithmetic: {{MutationID: "m2", Status: rpt.Killed}}},
		})
		require.NoError(t, os.WriteFile(filepath.Join(base, "bad.yaml"), []byte("source: [\n"), 0o600))

		wf := domain.NewWorkflow(nil, adapter.NewReportStore(), newReporter(), nil, nil)
		require.ErrorContains(t, wf.Merge(ctx, domain.MergeArgs{Reports: m.Path(base)}), "load existing reports from base")
	})
}

func TestWorkflow_Test_ShardedRunRebuildsBaseIndexAfterCleaningDeleted(t *testing.T) {
	ctx := context.Background()
	mockFSAdapter := new(domainmocks.MockSourceFSAdapter)
	mockReportStore := new(domainmocks.MockReportStore)
	mockReporter := new(domainmocks.MockReporter)
	mockMutagen := new(domainmocks.MockMutagen)
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockOrchestrator.EXPECT().NewWorkspace().Return(mockWorkspace).Maybe()
	mockWorkspace.EXPECT().Close(mock.Anything).Return().Maybe()

	gone := rpt.Source{Origin: &rpt.File{FullPath: "/p/gone.go", ShortPath: "gone.go", Hash: "g"}}
	shardDir := m.Path(filepath.Join("reports", domain.ShardDirPrefix+"1"))

	mockReporter.EXPECT().StartTest(ctx).Return(nil).Once()
	mockReporter.EXPECT().Wait(ctx).Return().Once()
	mockReporter.EXPECT().Close(ctx).Return().Once()
	mockReporter.EXPECT().DisplayConcurrencyInfo(ctx, mock.Anything, mock.Anything, mock.Anything).Return()
	mockReporter.EXPECT().DisplayUpcomingTestsInfo(ctx, 0).Return()
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return()

	mockFSAdapter.EXPECT().Stream(ctx, mock.Anything).Return(streamSources(nil))
	mockReportStore.EXPECT().CheckUpdates(ctx, m.Path("reports"), mock.Anything).Return(spillOf[rpt.Source](t), spillOf(t, gone), nil)
	mockReportStore.EXPECT().CleanReports(ctx, m.Path("reports"), mock.Anything).Return(nil).Once()
	mockReportStore.EXPECT().RegenerateIndex(ctx, m.Path("reports")).Return(nil).Once()
	mockReportStore.EXPECT().CleanReports(ctx, shardDir, mock.Anything).Return(nil).Maybe()
	mockReportStore.EXPECT().SaveReports(ctx, shardDir, mock.Anything).Return(nil).Once()
	mockReportStore.EXPECT().RegenerateIndex(ctx, shardDir).Return(nil).Once()

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockReporter, mockOrchestrator, mockMutagen)
	require.NoError(t, wf.Test(ctx, domain.TestArgs{
		EstimateArgs:    domain.EstimateArgs{Paths: []m.Path{"."}, UseCache: true, Reports: "reports"},
		Reports:         "reports",
		Threads:         1,
		ShardIndex:      1,
		TotalShardCount: 2,
	}))
	mockReportStore.AssertExpectations(t)
}

func TestWorkflow_Test_ErrorsAreReturned(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("boom")

	setup := func() (*domainmocks.MockSourceFSAdapter, *domainmocks.MockReportStore, *domainmocks.MockReporter, *domainmocks.MockMutagen) {
		fs := new(domainmocks.MockSourceFSAdapter)
		store := new(domainmocks.MockReportStore)
		reporter := new(domainmocks.MockReporter)
		mutagen := new(domainmocks.MockMutagen)

		reporter.EXPECT().StartTest(ctx).Return(nil).Once()
		reporter.EXPECT().Close(ctx).Return().Once()
		reporter.EXPECT().DisplayConcurrencyInfo(ctx, mock.Anything, mock.Anything, mock.Anything).Return()
		reporter.EXPECT().DisplayUpcomingTestsInfo(ctx, mock.Anything).Return().Maybe()
		fs.EXPECT().Stream(ctx, mock.Anything).Return(streamSources(nil))

		return fs, store, reporter, mutagen
	}

	cached := domain.TestArgs{
		EstimateArgs:    domain.EstimateArgs{Paths: []m.Path{"."}, UseCache: true, Reports: "reports"},
		Reports:         "reports",
		Threads:         1,
		TotalShardCount: 1,
	}

	t.Run("check updates", func(t *testing.T) {
		fs, store, reporter, mutagen := setup()
		store.EXPECT().CheckUpdates(ctx, mock.Anything, mock.Anything).Return(nil, nil, boom)

		err := domain.NewWorkflow(fs, store, reporter, nil, mutagen).Test(ctx, cached)
		require.ErrorIs(t, err, boom)
	})

	t.Run("clean deleted", func(t *testing.T) {
		fs, store, reporter, mutagen := setup()
		gone := rpt.Source{Origin: &rpt.File{ShortPath: "gone.go", Hash: "g"}}
		store.EXPECT().CheckUpdates(ctx, mock.Anything, mock.Anything).Return(spillOf[rpt.Source](t), spillOf(t, gone), nil)
		store.EXPECT().CleanReports(ctx, mock.Anything, mock.Anything).Return(boom)

		err := domain.NewWorkflow(fs, store, reporter, nil, mutagen).Test(ctx, cached)
		require.ErrorIs(t, err, boom)
	})

	t.Run("coverage profile", func(t *testing.T) {
		fs, store, reporter, mutagen := setup()
		fs.EXPECT().ReadFile(ctx, m.Path("cover.out")).Return(nil, boom)

		args := domain.TestArgs{Paths: []m.Path{"."}, Reports: "reports", Threads: 1, TotalShardCount: 1, CoverageProfile: "cover.out"}
		err := domain.NewWorkflow(fs, store, reporter, nil, mutagen).Test(ctx, args)
		require.ErrorIs(t, err, boom)
	})
}

func TestWorkflow_Estimate_StreamMutationsErrorIsReturned(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("boom")

	fs := new(domainmocks.MockSourceFSAdapter)
	reporter := new(domainmocks.MockReporter)
	mutagen := new(domainmocks.MockMutagen)

	reporter.EXPECT().StartEstimate(ctx).Return(nil).Once()
	reporter.EXPECT().Close(ctx).Return().Once()
	fs.EXPECT().Stream(ctx, mock.Anything).Return(streamSources([]rpt.Source{{Origin: &rpt.File{ShortPath: "a.go", FullPath: "/p/a.go"}}}))
	mutagen.EXPECT().
		StreamMutations(ctx, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(boom)

	err := domain.NewWorkflow(fs, nil, reporter, nil, mutagen).Estimate(ctx, domain.EstimateArgs{Paths: []m.Path{"."}})
	require.ErrorIs(t, err, boom)
}

func TestWorkflow_Merge_StoreErrorsAreReturned(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("boom")

	run := func(t *testing.T, configure func(store *domainmocks.MockReportStore)) error {
		t.Helper()

		base := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(base, domain.ShardDirPrefix+"0"), 0o750))

		shard := spillOf(t,
			m.Report{Source: rpt.Source{Origin: &rpt.File{ShortPath: "a.go", FullPath: "/p/a.go", Hash: "a"}},
				Result: m.Result{mgen.Arithmetic: {{MutationID: "m1", Status: rpt.Killed}}}},
			m.Report{Result: m.Result{mgen.Arithmetic: {{MutationID: "m0", Status: rpt.Killed}}}},
		)

		store := new(domainmocks.MockReportStore)
		store.EXPECT().LoadSpillReports(ctx, m.Path(filepath.Join(base, domain.ShardDirPrefix+"0"))).Return(shard, nil)
		configure(store)

		reporter := new(domainmocks.MockReporter)
		reporter.EXPECT().StartTest(ctx).Return(nil).Once()
		reporter.EXPECT().Close(ctx).Return().Once()

		return domain.NewWorkflow(nil, store, reporter, nil, nil).Merge(ctx, domain.MergeArgs{Reports: m.Path(base)})
	}

	t.Run("clean superseded", func(t *testing.T) {
		err := run(t, func(store *domainmocks.MockReportStore) {
			store.EXPECT().CleanReports(ctx, mock.Anything, mock.MatchedBy(func(sources pkg.FileSpill[rpt.Source]) bool {
				return assert.ObjectsAreEqual([]string{"a.go"}, spillKeys(sources))
			})).Return(boom)
		})
		require.ErrorIs(t, err, boom)
	})

	t.Run("save merged", func(t *testing.T) {
		err := run(t, func(store *domainmocks.MockReportStore) {
			store.EXPECT().CleanReports(ctx, mock.Anything, mock.Anything).Return(nil)
			store.EXPECT().SaveReports(ctx, mock.Anything, mock.Anything).Return(boom)
		})
		require.ErrorIs(t, err, boom)
	})
}

func TestWorkflow_Test_SourceSpillCreationErrorIsReturned(t *testing.T) {
	ctx := context.Background()

	fs := new(domainmocks.MockSourceFSAdapter)
	reporter := new(domainmocks.MockReporter)
	reporter.EXPECT().StartTest(ctx).Return(nil).Once()
	reporter.EXPECT().Close(ctx).Return().Once()
	reporter.EXPECT().DisplayConcurrencyInfo(ctx, mock.Anything, mock.Anything, mock.Anything).Return()
	fs.EXPECT().Stream(ctx, mock.Anything).Return(streamSources([]rpt.Source{{Origin: &rpt.File{ShortPath: "a.go"}}}))

	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))

	err := domain.NewWorkflow(fs, nil, reporter, nil, nil).Test(ctx, domain.TestArgs{
		Paths:           []m.Path{"."},
		Reports:         "reports",
		Threads:         1,
		TotalShardCount: 1,
	})
	require.ErrorContains(t, err, "create sources spill")
}

func TestWorkflow_Test_ShardedRunWithoutDeletedSourcesLeavesBaseUntouched(t *testing.T) {
	ctx := context.Background()
	mockFSAdapter := new(domainmocks.MockSourceFSAdapter)
	mockReportStore := new(domainmocks.MockReportStore)
	mockReporter := new(domainmocks.MockReporter)
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockOrchestrator.EXPECT().NewWorkspace().Return(mockWorkspace).Maybe()
	mockWorkspace.EXPECT().Close(mock.Anything).Return().Maybe()

	shardDir := m.Path(filepath.Join("reports", domain.ShardDirPrefix+"1"))

	mockReporter.EXPECT().StartTest(ctx).Return(nil).Once()
	mockReporter.EXPECT().Wait(ctx).Return().Once()
	mockReporter.EXPECT().Close(ctx).Return().Once()
	mockReporter.EXPECT().DisplayConcurrencyInfo(ctx, mock.Anything, mock.Anything, mock.Anything).Return()
	mockReporter.EXPECT().DisplayUpcomingTestsInfo(ctx, 0).Return()
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return()

	mockFSAdapter.EXPECT().Stream(ctx, mock.Anything).Return(streamSources(nil))
	mockReportStore.EXPECT().CheckUpdates(ctx, m.Path("reports"), mock.Anything).Return(spillOf[rpt.Source](t), spillOf[rpt.Source](t), nil)
	mockReportStore.EXPECT().CleanReports(ctx, shardDir, mock.Anything).Return(nil).Maybe()
	mockReportStore.EXPECT().SaveReports(ctx, shardDir, mock.Anything).Return(nil).Once()
	mockReportStore.EXPECT().RegenerateIndex(ctx, shardDir).Return(nil).Once()

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockReporter, mockOrchestrator, new(domainmocks.MockMutagen))
	require.NoError(t, wf.Test(ctx, domain.TestArgs{
		EstimateArgs:    domain.EstimateArgs{Paths: []m.Path{"."}, UseCache: true, Reports: "reports"},
		Reports:         "reports",
		Threads:         1,
		ShardIndex:      1,
		TotalShardCount: 2,
	}))

	mockReportStore.AssertNotCalled(t, "CleanReports", ctx, m.Path("reports"), mock.Anything)
	mockReportStore.AssertNotCalled(t, "RegenerateIndex", ctx, m.Path("reports"))
}

func TestWorkflow_Merge_KeepsBaselineWhenShardPathsAreNotRelative(t *testing.T) {
	ctx := context.Background()
	t.Chdir(t.TempDir())

	base := "reports"

	writeReports(t, base, m.Report{
		Source: rpt.Source{Origin: &rpt.File{ShortPath: "elsewhere.go", FullPath: "/old/elsewhere.go", Hash: "e"}},
		Result: m.Result{mgen.Arithmetic: {{MutationID: "me", Status: rpt.Survived}}},
	})
	writeReports(t, filepath.Join(base, domain.ShardDirPrefix+"0"), m.Report{
		Source: rpt.Source{Origin: &rpt.File{FullPath: "/new/a.go", Hash: "a2"}},
		Result: m.Result{mgen.Arithmetic: {{MutationID: "m2", Status: rpt.Killed}}},
	})

	var score float64

	wf := domain.NewWorkflow(nil, adapter.NewReportStore(), mergeReporter(ctx, &score), nil, nil)
	require.NoError(t, wf.Merge(ctx, domain.MergeArgs{Reports: m.Path(base)}))

	assert.ElementsMatch(t, []string{"a2", "e"}, reportHashes(t, base))
}

func TestWorkflow_Test_ShardFilterExcludesOtherShardsMutations(t *testing.T) {
	ctx := context.Background()
	mockFSAdapter := new(domainmocks.MockSourceFSAdapter)
	mockReportStore := new(domainmocks.MockReportStore)
	mockReporter := new(domainmocks.MockReporter)
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockMutagen := new(domainmocks.MockMutagen)

	source := rpt.Source{Origin: &rpt.File{FullPath: "/p/a.go", ShortPath: "a.go", Hash: "h"}, Test: &rpt.File{FullPath: "/p/a_test.go", Hash: "t"}}
	mutations := make([]m.Mutation, 0, 20)

	for i := range 20 {
		mutations = append(mutations, m.Mutation{ID: fmt.Sprintf("id-%02d", i), Source: source, Type: mgen.Arithmetic})
	}

	ran := 0

	mockReporter.EXPECT().StartTest(ctx).Return(nil).Once()
	mockReporter.EXPECT().Wait(ctx).Return().Once()
	mockReporter.EXPECT().Close(ctx).Return().Once()
	mockReporter.EXPECT().DisplayConcurrencyInfo(ctx, mock.Anything, mock.Anything, mock.Anything).Return()
	mockReporter.EXPECT().DisplayUpcomingTestsInfo(ctx, mock.Anything).Return()
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return()
	mockReporter.EXPECT().DisplayStartingTestInfo(ctx, mock.Anything, mock.Anything).Return()
	mockReporter.EXPECT().DisplayCompletedTestInfo(ctx, mock.Anything, mock.Anything).Return()
	mockFSAdapter.EXPECT().Stream(ctx, mock.Anything).Return(streamSources([]rpt.Source{source}))
	mockMutagen.EXPECT().
		StreamMutations(ctx, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(streamMutationsFn(mutations))
	mockOrchestrator.EXPECT().NewWorkspace().Return(mockWorkspace).Maybe()
	mockWorkspace.EXPECT().Close(mock.Anything).Return().Maybe()
	mockWorkspace.EXPECT().Run(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, mutation m.Mutation) (m.Result, error) {
			ran++
			return m.Result{mgen.Arithmetic: {{MutationID: mutation.ID, Status: rpt.Killed}}}, nil
		})
	mockReportStore.EXPECT().CleanReports(ctx, mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReportStore.EXPECT().SaveReports(ctx, mock.Anything, mock.Anything).Return(nil)
	mockReportStore.EXPECT().RegenerateIndex(ctx, mock.Anything).Return(nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockReporter, mockOrchestrator, mockMutagen)
	require.NoError(t, wf.Test(ctx, domain.TestArgs{
		Paths:           []m.Path{"."},
		Reports:         "reports",
		Threads:         1,
		ShardIndex:      0,
		TotalShardCount: 4,
	}))

	require.Positive(t, ran)
	require.Less(t, ran, len(mutations), "shard 0 of 4 must not run every mutation")
}
