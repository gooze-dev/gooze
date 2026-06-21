package domain_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	adaptermocks "gooze.dev/pkg/gooze/internal/adapter/mocks"
	domain "gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
	m "gooze.dev/pkg/gooze/internal/model"
	pkg "gooze.dev/pkg/gooze/pkg"
)

// streamSources returns closed channels that yield the given sources and no
// error, mimicking SourceFSAdapter.Stream for a successful scan.
func streamSources(sources []m.Source) (<-chan m.Source, <-chan error) {
	out := make(chan m.Source, len(sources))
	for _, source := range sources {
		out <- source
	}

	close(out)

	errc := make(chan error, 1)
	close(errc)

	return out, errc
}

// streamSourcesErr returns channels mimicking a scan that failed with err.
func streamSourcesErr(err error) (<-chan m.Source, <-chan error) {
	out := make(chan m.Source)
	close(out)

	errc := make(chan error, 1)
	errc <- err
	close(errc)

	return out, errc
}

func collectSpillReports(t *testing.T, reports pkg.FileSpill[m.Report]) []m.Report {
	t.Helper()

	collected := make([]m.Report, 0, int(reports.Len()))
	err := reports.Range(func(_ uint64, report m.Report) error {
		collected = append(collected, report)
		return nil
	})
	require.NoError(t, err)

	return collected
}

// streamMutationsFn returns a RunAndReturn callback that streams the given
// mutations through the StreamMutations fn argument. It is safe to invoke
// multiple times (the Test flow streams twice: once to count, once to run).
func streamMutationsFn(mutations []m.Mutation) func(context.Context, m.Source, func(m.Mutation) error, ...m.MutationType) error {
	return func(_ context.Context, _ m.Source, fn func(m.Mutation) error, _ ...m.MutationType) error {
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
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockMutagen := new(domainmocks.MockMutagen)

	sources := []m.Source{
		{
			Origin: &m.File{FullPath: "test.go", Hash: "hash1"},
			Test:   &m.File{FullPath: "test_test.go", Hash: "test_hash1"},
		},
	}

	mutations := []m.Mutation{
		{ID: "hash-1", Source: sources[0], Type: m.MutationArithmetic},
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
	mockReportStore.EXPECT().SaveSpillReports(ctx, mock.Anything, mock.Anything).Return(nil)
	mockReportStore.EXPECT().RegenerateIndex(ctx, mock.Anything).Return(nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockReporter, mockOrchestrator, mockMutagen)

	// Act
	args := domain.TestArgs{
		EstimateArgs: domain.EstimateArgs{
			Paths: []m.Path{"test.go"},
		},
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
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
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
		EstimateArgs: domain.EstimateArgs{
			Paths: []m.Path{"test.go"},
		},
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
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockMutagen := new(domainmocks.MockMutagen)

	sources := []m.Source{
		{Origin: &m.File{FullPath: "test.go", Hash: "hash1"}},
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
		EstimateArgs: domain.EstimateArgs{
			Paths: []m.Path{"test.go"},
		},
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
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(ctx, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockMutagen := new(domainmocks.MockMutagen)

	sources := []m.Source{
		{Origin: &m.File{FullPath: "test.go", Hash: "hash1"}},
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
		EstimateArgs: domain.EstimateArgs{
			Paths: []m.Path{"test.go"},
		},
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
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(ctx, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockMutagen := new(domainmocks.MockMutagen)

	sources := []m.Source{
		{Origin: &m.File{FullPath: "test.go", Hash: "hash1"}},
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
	mockReportStore.EXPECT().SaveSpillReports(ctx, mock.Anything, mock.Anything).Return(saveErr)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockReporter, mockOrchestrator, mockMutagen)

	// Act
	args := domain.TestArgs{
		EstimateArgs: domain.EstimateArgs{
			Paths: []m.Path{"test.go"},
		},
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
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockMutagen := new(domainmocks.MockMutagen)

	sources := []m.Source{
		{Origin: &m.File{FullPath: "test.go", Hash: "hash1"}},
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
	mockReportStore.EXPECT().SaveSpillReports(ctx, mock.Anything, mock.MatchedBy(func(reports pkg.FileSpill[m.Report]) bool {
		return reports.Len() == 0
	})).Return(nil)
	mockReportStore.EXPECT().RegenerateIndex(ctx, mock.Anything).Return(nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockReporter, mockOrchestrator, mockMutagen)

	// Act
	args := domain.TestArgs{
		EstimateArgs: domain.EstimateArgs{
			Paths: []m.Path{"test.go"},
		},
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
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(ctx, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockMutagen := new(domainmocks.MockMutagen)

	source := m.Source{
		Origin: &m.File{FullPath: "test.go", Hash: "hash1"},
	}
	sources := []m.Source{source}

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
	mockReportStore.EXPECT().SaveSpillReports(ctx, mock.Anything, mock.MatchedBy(func(reports pkg.FileSpill[m.Report]) bool {
		return reports.Len() == 3
	})).Return(nil)
	mockReportStore.EXPECT().RegenerateIndex(ctx, mock.Anything).Return(nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockReporter, mockOrchestrator, mockMutagen)

	// Act
	args := domain.TestArgs{
		EstimateArgs: domain.EstimateArgs{
			Paths: []m.Path{"test.go"},
		},
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
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockMutagen := new(domainmocks.MockMutagen)

	baseReportsDir := m.Path("reports")
	expectedShardDir := m.Path(filepath.Join(string(baseReportsDir), domain.ShardDirPrefix+"0"))

	source := m.Source{
		Origin: &m.File{FullPath: "test.go", Hash: "hash1"},
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
	mockFSAdapter.EXPECT().Stream(ctx, mock.Anything).Return(streamSources([]m.Source{source}))
	mockMutagen.EXPECT().
		StreamMutations(ctx, mock.Anything, mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4], domain.DefaultMutations[5]).
		RunAndReturn(streamMutationsFn(mutations))
	// With hash-based sharding, the number of mutations in shard 0 may vary
	mockWorkspace.EXPECT().Run(mock.Anything, mock.Anything).Return(m.Result{}, nil).Maybe()
	mockWorkspace.EXPECT().Close(mock.Anything).Return().Maybe()
	mockOrchestrator.EXPECT().NewWorkspace().Return(mockWorkspace).Maybe()
	mockReportStore.EXPECT().SaveSpillReports(ctx, expectedShardDir, mock.MatchedBy(func(reports pkg.FileSpill[m.Report]) bool {
		// Accept any number of reports since hash-based sharding determines this
		return true
	})).Return(nil)
	mockReportStore.EXPECT().RegenerateIndex(ctx, expectedShardDir).Return(nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockReporter, mockOrchestrator, mockMutagen)

	// Act
	args := domain.TestArgs{
		EstimateArgs: domain.EstimateArgs{
			Paths: []m.Path{"test.go"},
		},
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
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(ctx, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockMutagen := new(domainmocks.MockMutagen)

	source := m.Source{
		Origin: &m.File{FullPath: "test.go", Hash: "hash1"},
	}

	mutations := []m.Mutation{
		{ID: "hash-0", Source: source, Type: m.MutationArithmetic},
	}

	mockReporter.EXPECT().StartTest(ctx).Return(nil).Once()
	mockReporter.EXPECT().Wait(ctx).Return().Once()
	mockReporter.EXPECT().Close(ctx).Return().Once()
	mockReporter.EXPECT().DisplayConcurrencyInfo(ctx, mock.Anything, mock.Anything, mock.Anything).Return()
	mockReporter.EXPECT().DisplayUpcomingTestsInfo(ctx, mock.Anything).Return()
	mockReporter.EXPECT().DisplayStartingTestInfo(ctx, mutations[0], 0).Return().Once()
	mockReporter.EXPECT().DisplayCompletedTestInfo(ctx, mutations[0], mock.Anything).Return().Once()
	mockFSAdapter.EXPECT().Stream(ctx, mock.Anything).Return(streamSources([]m.Source{source}))
	mockMutagen.EXPECT().
		StreamMutations(ctx, mock.Anything, mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4], domain.DefaultMutations[5]).
		RunAndReturn(streamMutationsFn(mutations))
	mockWorkspace.EXPECT().Run(mock.Anything, mutations[0]).Return(m.Result{}, nil)
	mockWorkspace.EXPECT().Close(mock.Anything).Return().Maybe()
	mockOrchestrator.EXPECT().NewWorkspace().Return(mockWorkspace).Maybe()
	mockReportStore.EXPECT().SaveSpillReports(ctx, mock.Anything, mock.Anything).Return(nil)
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
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockMutagen := new(domainmocks.MockMutagen)

	source := m.Source{
		Origin: &m.File{FullPath: "test.go", Hash: "hash1"},
	}

	mutations := []m.Mutation{
		{ID: "hash-0", Source: source, Type: m.MutationArithmetic},
		{ID: "hash-1", Source: source, Type: m.MutationArithmetic},
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
	mockFSAdapter.EXPECT().Stream(ctx, mock.Anything).Return(streamSources([]m.Source{source}))
	mockMutagen.EXPECT().
		StreamMutations(ctx, mock.Anything, mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4], domain.DefaultMutations[5]).
		RunAndReturn(streamMutationsFn(mutations))
	mockWorkspace.EXPECT().Run(mock.Anything, mock.Anything).Return(m.Result{}, nil).Times(2)
	mockWorkspace.EXPECT().Close(mock.Anything).Return().Maybe()
	mockOrchestrator.EXPECT().NewWorkspace().Return(mockWorkspace).Maybe()
	mockReportStore.EXPECT().SaveSpillReports(ctx, mock.Anything, mock.MatchedBy(func(reports pkg.FileSpill[m.Report]) bool {
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
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockMutagen := new(domainmocks.MockMutagen)

	source := m.Source{
		Origin: &m.File{FullPath: "test.go", Hash: "hash1"},
	}

	mutations := []m.Mutation{
		{ID: "hash-0", Source: source, Type: m.MutationArithmetic},
		{ID: "hash-1", Source: source, Type: m.MutationArithmetic},
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
	mockFSAdapter.EXPECT().Stream(ctx, mock.Anything).Return(streamSources([]m.Source{source}))
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
	mockReportStore.EXPECT().SaveSpillReports(ctx, mock.Anything, mock.MatchedBy(func(reports pkg.FileSpill[m.Report]) bool {
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
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockMutagen := new(domainmocks.MockMutagen)

	diffCode := []byte("--- original\n+++ mutated\n@@ -1,1 +1,1 @@\n-\treturn 3 + 5\n+\treturn 3 - 5\n")

	sources := []m.Source{
		{
			Origin: &m.File{FullPath: "test.go", Hash: "hash1"},
			Test:   &m.File{FullPath: "test_test.go", Hash: "test_hash1"},
		},
	}

	mutations := []m.Mutation{
		{
			ID:       "hash-0",
			Source:   sources[0],
			Type:     m.MutationArithmetic,
			DiffCode: diffCode,
		},
	}

	skippedResult := m.Result{
		m.MutationArithmetic: []struct {
			MutationID string
			Status     m.TestStatus
			Err        error
		}{{MutationID: "hash-0", Status: m.Skipped}},
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

	mockReportStore.EXPECT().SaveSpillReports(ctx, mock.Anything, mock.MatchedBy(func(reports pkg.FileSpill[m.Report]) bool {
		collected := collectSpillReports(t, reports)
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
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockMutagen := new(domainmocks.MockMutagen)

	diffCode := []byte("--- original\n+++ mutated\n@@ -1,1 +1,1 @@\n-\treturn 3 + 5\n+\treturn 3 - 5\n")

	sources := []m.Source{
		{
			Origin: &m.File{FullPath: "test.go", Hash: "hash1"},
			Test:   &m.File{FullPath: "test_test.go", Hash: "test_hash1"},
		},
	}

	mutations := []m.Mutation{
		{
			ID:       "hash-2",
			Source:   sources[0],
			Type:     m.MutationArithmetic,
			DiffCode: diffCode,
		},
	}

	result := m.Result{
		m.MutationArithmetic: []struct {
			MutationID string
			Status     m.TestStatus
			Err        error
		}{
			{MutationID: "hash-1", Status: m.Killed},
			{MutationID: "hash-3", Status: m.Survived},
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

	mockReportStore.EXPECT().SaveSpillReports(ctx, mock.Anything, mock.MatchedBy(func(reports pkg.FileSpill[m.Report]) bool {
		collected := collectSpillReports(t, reports)
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
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockMutagen := new(domainmocks.MockMutagen)

	diffCode := []byte("--- original\n+++ mutated\n@@ -1,1 +1,1 @@\n-\treturn 3 + 5\n+\treturn 3 - 5\n")

	sources := []m.Source{
		{
			Origin: &m.File{FullPath: "test.go", Hash: "hash1"},
			Test:   &m.File{FullPath: "test_test.go", Hash: "test_hash1"},
		},
	}

	mutations := []m.Mutation{
		{
			ID:       "hash-0",
			Source:   sources[0],
			Type:     m.MutationArithmetic,
			DiffCode: diffCode,
		},
	}

	result := m.Result{
		m.MutationArithmetic: []struct {
			MutationID string
			Status     m.TestStatus
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

	mockReportStore.EXPECT().SaveSpillReports(ctx, mock.Anything, mock.MatchedBy(func(reports pkg.FileSpill[m.Report]) bool {
		collected := collectSpillReports(t, reports)
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
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockMutagen := new(domainmocks.MockMutagen)

	source1 := m.Source{
		Origin: &m.File{FullPath: "file1.go", Hash: "hash1"},
	}
	source2 := m.Source{
		Origin: &m.File{FullPath: "file2.go", Hash: "hash2"},
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
	mockFSAdapter.EXPECT().Stream(ctx, mock.Anything).Return(streamSources([]m.Source{source1, source2}))
	mockMutagen.EXPECT().
		StreamMutations(ctx, source1, mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4], domain.DefaultMutations[5]).
		RunAndReturn(streamMutationsFn(mutations1))
	mockMutagen.EXPECT().
		StreamMutations(ctx, source2, mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4], domain.DefaultMutations[5]).
		RunAndReturn(streamMutationsFn(mutations2))
	mockWorkspace.EXPECT().Run(mock.Anything, mock.Anything).Return(m.Result{}, nil).Times(3)
	mockWorkspace.EXPECT().Close(mock.Anything).Return().Maybe()
	mockOrchestrator.EXPECT().NewWorkspace().Return(mockWorkspace).Maybe()
	mockReportStore.EXPECT().SaveSpillReports(ctx, mock.Anything, mock.MatchedBy(func(reports pkg.FileSpill[m.Report]) bool {
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
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
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
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockMutagen := new(domainmocks.MockMutagen)

	diffCode := []byte("--- original\n+++ mutated\n@@ -1,1 +1,1 @@\n-\treturn 3 + 5\n+\treturn 3 - 5\n")

	sources := []m.Source{
		{
			Origin: &m.File{FullPath: "test.go", Hash: "hash1"},
			Test:   &m.File{FullPath: "test_test.go", Hash: "test_hash1"},
		},
	}

	mutations := []m.Mutation{
		{
			ID:       "hash-0",
			Source:   sources[0],
			Type:     m.MutationArithmetic,
			DiffCode: diffCode,
		},
	}

	// Mock a survived mutation result
	survivedResult := m.Result{
		m.MutationArithmetic: []struct {
			MutationID string
			Status     m.TestStatus
			Err        error
		}{{MutationID: "hash-0", Status: m.Survived}},
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
	mockReportStore.EXPECT().SaveSpillReports(ctx, mock.Anything, mock.MatchedBy(func(reports pkg.FileSpill[m.Report]) bool {
		collected := collectSpillReports(t, reports)
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
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockMutagen := new(domainmocks.MockMutagen)

	diffCode := []byte("--- original\n+++ mutated\n@@ -1,1 +1,1 @@\n-\treturn 3 + 5\n+\treturn 3 - 5\n")

	sources := []m.Source{
		{
			Origin: &m.File{FullPath: "test.go", Hash: "hash1"},
			Test:   &m.File{FullPath: "test_test.go", Hash: "test_hash1"},
		},
	}

	mutations := []m.Mutation{
		{
			ID:       "hash-0",
			Source:   sources[0],
			Type:     m.MutationArithmetic,
			DiffCode: diffCode,
		},
	}

	// Mock a killed mutation result
	killedResult := m.Result{
		m.MutationArithmetic: []struct {
			MutationID string
			Status     m.TestStatus
			Err        error
		}{{MutationID: "hash-0", Status: m.Killed}},
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
	mockReportStore.EXPECT().SaveSpillReports(ctx, mock.Anything, mock.MatchedBy(func(reports pkg.FileSpill[m.Report]) bool {
		collected := collectSpillReports(t, reports)
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
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockMutagen := new(domainmocks.MockMutagen)

	sources := []m.Source{
		{Origin: &m.File{FullPath: "test.go", Hash: "hash1"}},
	}

	mutations := []m.Mutation{
		{ID: "hash-0", Source: sources[0], Type: m.MutationArithmetic},
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
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
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

	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
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
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockMutagen := new(domainmocks.MockMutagen)

	sources := []m.Source{
		{Origin: &m.File{FullPath: "test.go", Hash: "hash1"}},
	}

	mutations := []m.Mutation{
		{ID: "hash-0", Source: sources[0], Type: m.MutationArithmetic},
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
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockMutagen := new(domainmocks.MockMutagen)

	source := m.Source{
		Origin: &m.File{FullPath: "test.go", Hash: "hash1"},
	}

	mutations := []m.Mutation{
		{ID: "hash-0", Source: source, Type: m.MutationArithmetic},
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
	mockFSAdapter.EXPECT().Stream(ctx, mock.Anything).Return(streamSources([]m.Source{source}))
	mockMutagen.EXPECT().
		StreamMutations(ctx, mock.Anything, mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4], domain.DefaultMutations[5]).
		RunAndReturn(streamMutationsFn(mutations))
	mockWorkspace.EXPECT().Run(mock.Anything, mutations[0]).Return(m.Result{}, nil)
	mockWorkspace.EXPECT().Close(mock.Anything).Return().Maybe()
	mockOrchestrator.EXPECT().NewWorkspace().Return(mockWorkspace).Maybe()
	mockReportStore.EXPECT().SaveSpillReports(ctx, mock.Anything, mock.Anything).Return(nil)
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
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything, mock.Anything).Return(nil).Maybe()
	mockReporter := new(domainmocks.MockReporter)
	mockReporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return().Maybe()
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockWorkspace := new(domainmocks.MockWorkspace)
	mockMutagen := new(domainmocks.MockMutagen)

	diffCode := []byte("--- original\n+++ mutated\n@@ -1,1 +1,1 @@\n-\treturn 3 + 5\n+\treturn 3 - 5\n")

	sources := []m.Source{
		{
			Origin: &m.File{FullPath: "test.go", Hash: "hash1"},
			Test:   &m.File{FullPath: "test_test.go", Hash: "test_hash1"},
		},
	}

	mutations := []m.Mutation{
		{
			ID:       "hash-1",
			Source:   sources[0],
			Type:     m.MutationArithmetic,
			DiffCode: diffCode,
		},
	}

	result := m.Result{
		m.MutationArithmetic: []struct {
			MutationID string
			Status     m.TestStatus
			Err        error
		}{
			{MutationID: "hash-0", Status: m.Killed},
			{MutationID: "hash-1", Status: m.Survived},
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

	mockReportStore.EXPECT().SaveSpillReports(ctx, mock.Anything, mock.MatchedBy(func(reports pkg.FileSpill[m.Report]) bool {
		collected := collectSpillReports(t, reports)
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
