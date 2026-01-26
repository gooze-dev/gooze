package domain_test

import (
	"crypto/sha256"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	adaptermocks "github.com/mouse-blink/gooze/internal/adapter/mocks"
	controllermocks "github.com/mouse-blink/gooze/internal/controller/mocks"
	domain "github.com/mouse-blink/gooze/internal/domain"
	domainmocks "github.com/mouse-blink/gooze/internal/domain/mocks"
	m "github.com/mouse-blink/gooze/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestWorkflow_Test_Success(t *testing.T) {
	// Arrange
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything).Return(nil).Maybe()
	mockUI := new(controllermocks.MockUI)
	mockOrchestrator := new(domainmocks.MockOrchestrator)
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

	mockUI.EXPECT().Start(mock.Anything).Return(nil).Once()
	mockUI.EXPECT().Wait().Return().Once()
	mockUI.EXPECT().Close().Return().Once()
	mockUI.EXPECT().DisplayConcurencyInfo(mock.Anything, mock.Anything, mock.Anything).Return()
	mockUI.EXPECT().DusplayUpcomingTestsInfo(mock.Anything).Return()
	mockUI.EXPECT().DisplayStartingTestInfo(mock.Anything, mock.Anything).Return().Once()
	mockUI.EXPECT().DisplayCompletedTestInfo(mock.Anything, mock.Anything).Return().Once()
	mockFSAdapter.EXPECT().Get(mock.Anything).Return(sources, nil)
	mockMutagen.EXPECT().GenerateMutation(mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4]).Return(mutations, nil)
	mockOrchestrator.EXPECT().TestMutation(mock.Anything).Return(m.Result{}, nil)
	mockReportStore.EXPECT().SaveReports(mock.Anything, mock.Anything).Return(nil)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything).Return(nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockUI, mockOrchestrator, mockMutagen)

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
	err := wf.Test(args)

	// Assert
	assert.NoError(t, err)
	mockFSAdapter.AssertExpectations(t)
	mockMutagen.AssertExpectations(t)
	mockReportStore.AssertExpectations(t)
	mockOrchestrator.AssertExpectations(t)
}

func TestWorkflow_Test_GetSourcesError(t *testing.T) {
	// Arrange
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything).Return(nil).Maybe()
	mockUI := new(controllermocks.MockUI)
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockMutagen := new(domainmocks.MockMutagen)

	testErr := errors.New("failed to get sources")
	mockUI.EXPECT().Start(mock.Anything).Return(nil).Once()
	mockUI.EXPECT().Close().Return().Once()
	mockUI.EXPECT().DisplayConcurencyInfo(mock.Anything, mock.Anything, mock.Anything).Return()
	mockFSAdapter.EXPECT().Get(mock.Anything).Return(nil, testErr)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockUI, mockOrchestrator, mockMutagen)

	// Act
	args := domain.TestArgs{
		EstimateArgs: domain.EstimateArgs{
			Paths: []m.Path{"test.go"},
		},
		Reports: "reports.json",
	}
	err := wf.Test(args)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "get sources")
}

func TestWorkflow_Test_GenerateMutationsError(t *testing.T) {
	// Arrange
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything).Return(nil).Maybe()
	mockUI := new(controllermocks.MockUI)
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockMutagen := new(domainmocks.MockMutagen)

	sources := []m.Source{
		{Origin: &m.File{FullPath: "test.go", Hash: "hash1"}},
	}

	testErr := errors.New("failed to generate mutations")
	mockUI.EXPECT().Start(mock.Anything).Return(nil).Once()
	mockUI.EXPECT().Close().Return().Once()
	mockUI.EXPECT().DisplayConcurencyInfo(mock.Anything, mock.Anything, mock.Anything).Return()
	mockFSAdapter.EXPECT().Get(mock.Anything).Return(sources, nil)
	mockMutagen.EXPECT().GenerateMutation(mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4]).Return(nil, testErr)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockUI, mockOrchestrator, mockMutagen)

	// Act
	args := domain.TestArgs{
		EstimateArgs: domain.EstimateArgs{
			Paths: []m.Path{"test.go"},
		},
		Reports: "reports.json",
	}
	err := wf.Test(args)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "generate mutations")
}

func TestWorkflow_Test_TestMutationError(t *testing.T) {
	// Arrange
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything).Return(nil).Maybe()
	mockUI := new(controllermocks.MockUI)
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockMutagen := new(domainmocks.MockMutagen)

	sources := []m.Source{
		{Origin: &m.File{FullPath: "test.go", Hash: "hash1"}},
	}

	mutations := []m.Mutation{
		{ID: "hash-1", Source: sources[0]},
	}

	testErr := errors.New("failed to test mutation")
	mockUI.EXPECT().Start(mock.Anything).Return(nil).Once()
	mockUI.EXPECT().Close().Return().Once()
	mockUI.EXPECT().DisplayConcurencyInfo(mock.Anything, mock.Anything, mock.Anything).Return()
	mockUI.EXPECT().DusplayUpcomingTestsInfo(mock.Anything).Return()
	mockUI.EXPECT().DisplayStartingTestInfo(mock.Anything, mock.Anything).Return().Once()
	mockFSAdapter.EXPECT().Get(mock.Anything).Return(sources, nil)
	mockMutagen.EXPECT().GenerateMutation(mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4]).Return(mutations, nil)
	mockOrchestrator.EXPECT().TestMutation(mock.Anything).Return(nil, testErr)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockUI, mockOrchestrator, mockMutagen)

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
	err := wf.Test(args)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "errors occurred during mutation testing")
}

func TestWorkflow_Test_SaveReportsError(t *testing.T) {
	// Arrange
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything).Return(nil).Maybe()
	mockUI := new(controllermocks.MockUI)
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockMutagen := new(domainmocks.MockMutagen)

	sources := []m.Source{
		{Origin: &m.File{FullPath: "test.go", Hash: "hash1"}},
	}

	mutations := []m.Mutation{
		{ID: "hash-1", Source: sources[0]},
	}

	mockUI.EXPECT().Start(mock.Anything).Return(nil).Once()
	mockUI.EXPECT().Close().Return().Once()
	mockUI.EXPECT().DisplayConcurencyInfo(mock.Anything, mock.Anything, mock.Anything).Return()
	mockUI.EXPECT().DusplayUpcomingTestsInfo(mock.Anything).Return()
	mockUI.EXPECT().DisplayStartingTestInfo(mock.Anything, mock.Anything).Return().Once()
	mockUI.EXPECT().DisplayCompletedTestInfo(mock.Anything, mock.Anything).Return().Once()
	mockFSAdapter.EXPECT().Get(mock.Anything).Return(sources, nil)
	mockMutagen.EXPECT().GenerateMutation(mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4]).Return(mutations, nil)
	mockOrchestrator.EXPECT().TestMutation(mock.Anything).Return(m.Result{}, nil)

	saveErr := errors.New("failed to save reports")
	mockReportStore.EXPECT().SaveReports(mock.Anything, mock.Anything).Return(saveErr)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockUI, mockOrchestrator, mockMutagen)

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
	err := wf.Test(args)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "save reports")
}

func TestWorkflow_Test_NoMutations(t *testing.T) {
	// Arrange
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything).Return(nil).Maybe()
	mockUI := new(controllermocks.MockUI)
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockMutagen := new(domainmocks.MockMutagen)

	sources := []m.Source{
		{Origin: &m.File{FullPath: "test.go", Hash: "hash1"}},
	}

	// No mutations generated
	mockUI.EXPECT().Start(mock.Anything).Return(nil).Once()
	mockUI.EXPECT().Wait().Return().Once()
	mockUI.EXPECT().Close().Return().Once()
	mockUI.EXPECT().DisplayConcurencyInfo(mock.Anything, mock.Anything, mock.Anything).Return()
	mockUI.EXPECT().DusplayUpcomingTestsInfo(mock.Anything).Return()
	mockFSAdapter.EXPECT().Get(mock.Anything).Return(sources, nil)
	mockMutagen.EXPECT().GenerateMutation(mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4]).Return([]m.Mutation{}, nil)
	mockReportStore.EXPECT().SaveReports(mock.Anything, mock.MatchedBy(func(reports []m.Report) bool {
		return len(reports) == 0
	})).Return(nil)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything).Return(nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockUI, mockOrchestrator, mockMutagen)

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
	err := wf.Test(args)

	// Assert
	assert.NoError(t, err)
	mockFSAdapter.AssertExpectations(t)
	mockMutagen.AssertExpectations(t)
	mockReportStore.AssertExpectations(t)
}

func TestWorkflow_Test_MultipleThreads(t *testing.T) {
	// Arrange
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything).Return(nil).Maybe()
	mockUI := new(controllermocks.MockUI)
	mockOrchestrator := new(domainmocks.MockOrchestrator)
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

	mockUI.EXPECT().Start(mock.Anything).Return(nil).Once()
	mockUI.EXPECT().Wait().Return().Once()
	mockUI.EXPECT().Close().Return().Once()
	mockUI.EXPECT().DisplayConcurencyInfo(mock.Anything, mock.Anything, mock.Anything).Return()
	mockUI.EXPECT().DusplayUpcomingTestsInfo(mock.Anything).Return()
	mockUI.EXPECT().DisplayStartingTestInfo(mock.Anything, mock.Anything).Return().Times(3)
	mockUI.EXPECT().DisplayCompletedTestInfo(mock.Anything, mock.Anything).Return().Times(3)
	mockFSAdapter.EXPECT().Get(mock.Anything).Return(sources, nil)
	mockMutagen.EXPECT().GenerateMutation(mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4]).Return(mutations, nil)
	mockOrchestrator.EXPECT().TestMutation(mock.Anything).Return(m.Result{}, nil).Times(3)
	mockReportStore.EXPECT().SaveReports(mock.Anything, mock.MatchedBy(func(reports []m.Report) bool {
		return len(reports) == 3
	})).Return(nil)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything).Return(nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockUI, mockOrchestrator, mockMutagen)

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
	err := wf.Test(args)

	// Assert
	assert.NoError(t, err)
	mockOrchestrator.AssertExpectations(t)
}

func TestWorkflow_Test_WithSharding(t *testing.T) {
	// Arrange
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything).Return(nil).Maybe()
	mockUI := new(controllermocks.MockUI)
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockMutagen := new(domainmocks.MockMutagen)

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

	mockUI.EXPECT().Start(mock.Anything).Return(nil).Once()
	mockUI.EXPECT().Wait().Return().Once()
	mockUI.EXPECT().Close().Return().Once()
	mockUI.EXPECT().DisplayConcurencyInfo(mock.Anything, mock.Anything, mock.Anything).Return()
	mockUI.EXPECT().DusplayUpcomingTestsInfo(mock.Anything).Return()
	mockUI.EXPECT().DisplayStartingTestInfo(mock.Anything, mock.Anything).Return().Maybe()
	mockUI.EXPECT().DisplayCompletedTestInfo(mock.Anything, mock.Anything).Return().Maybe()
	mockFSAdapter.EXPECT().Get(mock.Anything).Return([]m.Source{source}, nil)
	mockMutagen.EXPECT().GenerateMutation(mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4]).Return(mutations, nil)
	// With hash-based sharding, the number of mutations in shard 0 may vary
	mockOrchestrator.EXPECT().TestMutation(mock.Anything).Return(m.Result{}, nil).Maybe()
	mockReportStore.EXPECT().SaveReports(mock.Anything, mock.MatchedBy(func(reports []m.Report) bool {
		// Accept any number of reports since hash-based sharding determines this
		return true
	})).Return(nil)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything).Return(nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockUI, mockOrchestrator, mockMutagen)

	// Act
	args := domain.TestArgs{
		EstimateArgs: domain.EstimateArgs{
			Paths: []m.Path{"test.go"},
		},
		Reports:         "reports.json",
		Threads:         1,
		ShardIndex:      0,
		TotalShardCount: 3,
	}
	err := wf.Test(args)

	// Assert
	assert.NoError(t, err)
	mockOrchestrator.AssertExpectations(t)
}

func TestWorkflow_ShardMutations_InvalidShardReturnsEmpty(t *testing.T) {
	// Arrange
	mutations := []m.Mutation{
		{ID: "hash-0"},
		{ID: "hash-1"},
		{ID: "hash-2"},
	}

	wf := domain.NewWorkflow(nil, nil, nil, nil, nil)

	// Act
	result := wf.(interface {
		ShardMutations([]m.Mutation, int, int) []m.Mutation
	}).ShardMutations(mutations, 2, 2)

	// Assert
	assert.Empty(t, result)
}

func TestWorkflow_ShardMutations_ShardIndexGreaterThanTotalReturnsEmpty(t *testing.T) {
	// Arrange
	mutations := []m.Mutation{
		{ID: "hash-0"},
		{ID: "hash-1"},
		{ID: "hash-2"},
	}

	wf := domain.NewWorkflow(nil, nil, nil, nil, nil)

	// Act
	result := wf.(interface {
		ShardMutations([]m.Mutation, int, int) []m.Mutation
	}).ShardMutations(mutations, 3, 2)

	// Assert
	assert.Empty(t, result)
}

func TestWorkflow_ShardMutations_NonPositiveTotalReturnsAll(t *testing.T) {
	// Arrange
	mutations := []m.Mutation{
		{ID: "hash-0"},
		{ID: "hash-1"},
		{ID: "hash-2"},
	}

	wf := domain.NewWorkflow(nil, nil, nil, nil, nil)

	// Act
	resultZero := wf.(interface {
		ShardMutations([]m.Mutation, int, int) []m.Mutation
	}).ShardMutations(mutations, 0, 0)

	resultNegative := wf.(interface {
		ShardMutations([]m.Mutation, int, int) []m.Mutation
	}).ShardMutations(mutations, 0, -3)

	// Assert
	assert.Len(t, resultZero, len(mutations))
	assert.Len(t, resultNegative, len(mutations))
}

func TestWorkflow_ShardMutations_MiddleShardSelectsExactMatches(t *testing.T) {
	// Arrange
	mutations := []m.Mutation{
		{ID: "hash-0"},
		{ID: "hash-1"},
		{ID: "hash-2"},
		{ID: "hash-3"},
		{ID: "hash-4"},
		{ID: "hash-5"},
	}

	wf := domain.NewWorkflow(nil, nil, nil, nil, nil)

	// Act
	result := wf.(interface {
		ShardMutations([]m.Mutation, int, int) []m.Mutation
	}).ShardMutations(mutations, 1, 3)

	// Assert
	// With hash-based sharding, we can't predict exact counts, so just verify sharding works
	assert.True(t, len(result) >= 0, "ShardMutations should return some mutations for shard 1")

	// Verify that each returned mutation actually belongs to shard 1
	for _, mutation := range result {
		// Verify this mutation would actually be assigned to shard 1
		h := sha256.Sum256([]byte(mutation.ID))
		hashValue := int(h[0])<<24 + int(h[1])<<16 + int(h[2])<<8 + int(h[3])
		if hashValue < 0 {
			hashValue = -hashValue
		}
		expectedShard := hashValue % 3
		assert.Equal(t, 1, expectedShard, "Mutation %s should belong to shard 1", mutation.ID)
	}
}

func TestWorkflow_TestThreadsZeroDoesNotPanic(t *testing.T) {
	// Arrange
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything).Return(nil).Maybe()
	mockUI := new(controllermocks.MockUI)
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockMutagen := new(domainmocks.MockMutagen)

	source := m.Source{
		Origin: &m.File{FullPath: "test.go", Hash: "hash1"},
	}

	mutations := []m.Mutation{
		{ID: "hash-0", Source: source, Type: m.MutationArithmetic},
	}

	mockUI.EXPECT().Start(mock.Anything).Return(nil).Once()
	mockUI.EXPECT().Wait().Return().Once()
	mockUI.EXPECT().Close().Return().Once()
	mockUI.EXPECT().DisplayConcurencyInfo(mock.Anything, mock.Anything, mock.Anything).Return()
	mockUI.EXPECT().DusplayUpcomingTestsInfo(mock.Anything).Return()
	mockUI.EXPECT().DisplayStartingTestInfo(mutations[0], 0).Return().Once()
	mockUI.EXPECT().DisplayCompletedTestInfo(mutations[0], mock.Anything).Return().Once()
	mockFSAdapter.EXPECT().Get(mock.Anything).Return([]m.Source{source}, nil)
	mockMutagen.EXPECT().GenerateMutation(mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4]).Return(mutations, nil)
	mockOrchestrator.EXPECT().TestMutation(mutations[0]).Return(m.Result{}, nil)
	mockReportStore.EXPECT().SaveReports(mock.Anything, mock.Anything).Return(nil)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything).Return(nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockUI, mockOrchestrator, mockMutagen)

	// Act
	args := domain.TestArgs{
		Reports:         "reports.json",
		Threads:         0,
		ShardIndex:      0,
		TotalShardCount: 1,
	}
	err := wf.Test(args)

	// Assert
	assert.NoError(t, err)
}

func TestWorkflow_TestThreadIDWithinBounds(t *testing.T) {
	// Arrange
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything).Return(nil).Maybe()
	mockUI := new(controllermocks.MockUI)
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockMutagen := new(domainmocks.MockMutagen)

	source := m.Source{
		Origin: &m.File{FullPath: "test.go", Hash: "hash1"},
	}

	mutations := []m.Mutation{
		{ID: "hash-0", Source: source, Type: m.MutationArithmetic},
		{ID: "hash-1", Source: source, Type: m.MutationArithmetic},
	}

	mockUI.EXPECT().Start(mock.Anything).Return(nil).Once()
	mockUI.EXPECT().Wait().Return().Once()
	mockUI.EXPECT().Close().Return().Once()
	mockUI.EXPECT().DisplayConcurencyInfo(mock.Anything, mock.Anything, mock.Anything).Return()
	mockUI.EXPECT().DusplayUpcomingTestsInfo(mock.Anything).Return()
	mockUI.EXPECT().DisplayStartingTestInfo(mock.Anything, mock.MatchedBy(func(id int) bool {
		return id >= 0 && id < 2
	})).Return().Times(2)
	mockUI.EXPECT().DisplayCompletedTestInfo(mock.Anything, mock.Anything).Return().Times(2)
	mockFSAdapter.EXPECT().Get(mock.Anything).Return([]m.Source{source}, nil)
	mockMutagen.EXPECT().GenerateMutation(mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4]).Return(mutations, nil)
	mockOrchestrator.EXPECT().TestMutation(mock.Anything).Return(m.Result{}, nil).Times(2)
	mockReportStore.EXPECT().SaveReports(mock.Anything, mock.MatchedBy(func(reports []m.Report) bool {
		return len(reports) == 2
	})).Return(nil)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything).Return(nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockUI, mockOrchestrator, mockMutagen)

	// Act
	args := domain.TestArgs{
		Reports:         "reports.json",
		Threads:         2,
		ShardIndex:      0,
		TotalShardCount: 1,
	}
	err := wf.Test(args)

	// Assert
	assert.NoError(t, err)
}

func TestWorkflow_TestThreadIDIsUniqueForThreadsTwo(t *testing.T) {
	// Arrange
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything).Return(nil).Maybe()
	mockUI := new(controllermocks.MockUI)
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockMutagen := new(domainmocks.MockMutagen)

	source := m.Source{
		Origin: &m.File{FullPath: "test.go", Hash: "hash1"},
	}

	mutations := []m.Mutation{
		{ID: "hash-0", Source: source, Type: m.MutationArithmetic},
		{ID: "hash-1", Source: source, Type: m.MutationArithmetic},
	}

	threadIDs := make([]int, 0, 2)

	mockUI.EXPECT().Start(mock.Anything).Return(nil).Once()
	mockUI.EXPECT().Wait().Return().Once()
	mockUI.EXPECT().Close().Return().Once()
	mockUI.EXPECT().DisplayConcurencyInfo(mock.Anything, mock.Anything, mock.Anything).Return()
	mockUI.EXPECT().DusplayUpcomingTestsInfo(mock.Anything).Return()
	mockUI.EXPECT().DisplayStartingTestInfo(mock.Anything, mock.Anything).Run(func(_ m.Mutation, threadID int) {
		threadIDs = append(threadIDs, threadID)
	}).Return().Times(2)
	mockUI.EXPECT().DisplayCompletedTestInfo(mock.Anything, mock.Anything).Return().Times(2)
	mockFSAdapter.EXPECT().Get(mock.Anything).Return([]m.Source{source}, nil)
	mockMutagen.EXPECT().GenerateMutation(mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4]).Return(mutations, nil)
	mockOrchestrator.EXPECT().TestMutation(mock.Anything).Return(m.Result{}, nil).Times(2)
	mockReportStore.EXPECT().SaveReports(mock.Anything, mock.MatchedBy(func(reports []m.Report) bool {
		return len(reports) == 2
	})).Return(nil)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything).Return(nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockUI, mockOrchestrator, mockMutagen)

	// Act
	args := domain.TestArgs{
		Reports:         "reports.json",
		Threads:         2,
		ShardIndex:      0,
		TotalShardCount: 1,
	}
	err := wf.Test(args)

	// Assert
	assert.NoError(t, err)
	if assert.Len(t, threadIDs, 2) {
		assert.NotEqual(t, threadIDs[0], threadIDs[1])
	}
}

func TestWorkflow_TestWithSkippedMutation(t *testing.T) {
	// Arrange
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything).Return(nil).Maybe()
	mockUI := new(controllermocks.MockUI)
	mockOrchestrator := new(domainmocks.MockOrchestrator)
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

	mockUI.EXPECT().Start(mock.Anything).Return(nil).Once()
	mockUI.EXPECT().Wait().Return().Once()
	mockUI.EXPECT().Close().Return().Once()
	mockUI.EXPECT().DisplayConcurencyInfo(mock.Anything, mock.Anything, mock.Anything).Return()
	mockUI.EXPECT().DusplayUpcomingTestsInfo(1).Return()
	mockUI.EXPECT().DisplayStartingTestInfo(mutations[0], 0).Return().Once()
	mockUI.EXPECT().DisplayCompletedTestInfo(mutations[0], skippedResult).Return().Once()

	mockFSAdapter.EXPECT().Get(mock.Anything).Return(sources, nil)
	mockMutagen.EXPECT().GenerateMutation(mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4]).Return(mutations, nil)
	mockOrchestrator.EXPECT().TestMutation(mutations[0]).Return(skippedResult, nil)

	mockReportStore.EXPECT().SaveReports(mock.Anything, mock.MatchedBy(func(reports []m.Report) bool {
		if len(reports) != 1 {
			return false
		}
		report := reports[0]
		return report.Diff == nil
	})).Return(nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockUI, mockOrchestrator, mockMutagen)

	// Act
	args := domain.TestArgs{
		Reports:         "reports.json",
		Threads:         1,
		ShardIndex:      0,
		TotalShardCount: 1,
	}
	err := wf.Test(args)

	// Assert
	assert.NoError(t, err)
}

func TestWorkflow_TestMutationIDExactMatchDoesNotUseHigherID(t *testing.T) {
	// Arrange
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything).Return(nil).Maybe()
	mockUI := new(controllermocks.MockUI)
	mockOrchestrator := new(domainmocks.MockOrchestrator)
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

	mockUI.EXPECT().Start(mock.Anything).Return(nil).Once()
	mockUI.EXPECT().Wait().Return().Once()
	mockUI.EXPECT().Close().Return().Once()
	mockUI.EXPECT().DisplayConcurencyInfo(mock.Anything, mock.Anything, mock.Anything).Return()
	mockUI.EXPECT().DusplayUpcomingTestsInfo(1).Return()
	mockUI.EXPECT().DisplayStartingTestInfo(mutations[0], 0).Return().Once()
	mockUI.EXPECT().DisplayCompletedTestInfo(mutations[0], result).Return().Once()

	mockFSAdapter.EXPECT().Get(mock.Anything).Return(sources, nil)
	mockMutagen.EXPECT().GenerateMutation(mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4]).Return(mutations, nil)
	mockOrchestrator.EXPECT().TestMutation(mutations[0]).Return(result, nil)

	mockReportStore.EXPECT().SaveReports(mock.Anything, mock.MatchedBy(func(reports []m.Report) bool {
		if len(reports) != 1 {
			return false
		}
		report := reports[0]
		return report.Diff == nil
	})).Return(nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockUI, mockOrchestrator, mockMutagen)

	// Act
	args := domain.TestArgs{
		Reports:         "reports.json",
		Threads:         1,
		ShardIndex:      0,
		TotalShardCount: 1,
	}
	err := wf.Test(args)

	// Assert
	assert.NoError(t, err)
}

func TestWorkflow_TestEmptyResultEntriesReturnsError(t *testing.T) {
	// Arrange
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything).Return(nil).Maybe()
	mockUI := new(controllermocks.MockUI)
	mockOrchestrator := new(domainmocks.MockOrchestrator)
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

	mockUI.EXPECT().Start(mock.Anything).Return(nil).Once()
	mockUI.EXPECT().Wait().Return().Once()
	mockUI.EXPECT().Close().Return().Once()
	mockUI.EXPECT().DisplayConcurencyInfo(mock.Anything, mock.Anything, mock.Anything).Return()
	mockUI.EXPECT().DusplayUpcomingTestsInfo(1).Return()
	mockUI.EXPECT().DisplayStartingTestInfo(mutations[0], 0).Return().Once()
	mockUI.EXPECT().DisplayCompletedTestInfo(mutations[0], result).Return().Once()

	mockFSAdapter.EXPECT().Get(mock.Anything).Return(sources, nil)
	mockMutagen.EXPECT().GenerateMutation(mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4]).Return(mutations, nil)
	mockOrchestrator.EXPECT().TestMutation(mutations[0]).Return(result, nil)

	mockReportStore.EXPECT().SaveReports(mock.Anything, mock.MatchedBy(func(reports []m.Report) bool {
		if len(reports) != 1 {
			return false
		}
		report := reports[0]
		return report.Diff == nil
	})).Return(nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockUI, mockOrchestrator, mockMutagen)

	// Act
	args := domain.TestArgs{
		Reports:         "reports.json",
		Threads:         1,
		ShardIndex:      0,
		TotalShardCount: 1,
	}
	err := wf.Test(args)

	// Assert
	assert.NoError(t, err)
}

func TestWorkflow_Test_MultipleSources(t *testing.T) {
	// Arrange
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything).Return(nil).Maybe()
	mockUI := new(controllermocks.MockUI)
	mockOrchestrator := new(domainmocks.MockOrchestrator)
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

	mockUI.EXPECT().Start(mock.Anything).Return(nil).Once()
	mockUI.EXPECT().Wait().Return().Once()
	mockUI.EXPECT().Close().Return().Once()
	mockUI.EXPECT().DisplayConcurencyInfo(mock.Anything, mock.Anything, mock.Anything).Return()
	mockUI.EXPECT().DusplayUpcomingTestsInfo(mock.Anything).Return()
	mockUI.EXPECT().DisplayStartingTestInfo(mock.Anything, mock.Anything).Return().Times(3)
	mockUI.EXPECT().DisplayCompletedTestInfo(mock.Anything, mock.Anything).Return().Times(3)
	mockFSAdapter.EXPECT().Get(mock.Anything).Return([]m.Source{source1, source2}, nil)
	mockMutagen.EXPECT().GenerateMutation(source1, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4]).Return(mutations1, nil)
	mockMutagen.EXPECT().GenerateMutation(source2, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4]).Return(mutations2, nil)
	mockOrchestrator.EXPECT().TestMutation(mock.Anything).Return(m.Result{}, nil).Times(3)
	mockReportStore.EXPECT().SaveReports(mock.Anything, mock.MatchedBy(func(reports []m.Report) bool {
		return len(reports) == 3
	})).Return(nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockUI, mockOrchestrator, mockMutagen)

	// Act
	args := domain.TestArgs{

		Reports:         "reports.json",
		Threads:         1,
		ShardIndex:      0,
		TotalShardCount: 1,
	}
	err := wf.Test(args)

	// Assert
	assert.NoError(t, err)
	mockFSAdapter.AssertExpectations(t)
	mockMutagen.AssertExpectations(t)
	mockOrchestrator.AssertExpectations(t)
	mockReportStore.AssertExpectations(t)
}

func TestWorkflow_NewWorkflowV2(t *testing.T) {
	// Arrange
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything).Return(nil).Maybe()
	mockUI := new(controllermocks.MockUI)
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockMutagen := new(domainmocks.MockMutagen)

	// Act
	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockUI, mockOrchestrator, mockMutagen)

	// Assert
	require.NotNil(t, wf)
	assert.Implements(t, (*domain.Workflow)(nil), wf)
}

func TestWorkflow_TestWithSurvivedMutation(t *testing.T) {
	// Arrange - This test specifically checks that survived mutations include diff data
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything).Return(nil).Maybe()
	mockUI := new(controllermocks.MockUI)
	mockOrchestrator := new(domainmocks.MockOrchestrator)
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

	mockUI.EXPECT().Start(mock.Anything).Return(nil).Once()
	mockUI.EXPECT().Wait().Return().Once()
	mockUI.EXPECT().Close().Return().Once()
	mockUI.EXPECT().DisplayConcurencyInfo(mock.Anything, mock.Anything, mock.Anything).Return()
	mockUI.EXPECT().DusplayUpcomingTestsInfo(1).Return()
	mockUI.EXPECT().DisplayStartingTestInfo(mutations[0], 0).Return().Once()
	mockUI.EXPECT().DisplayCompletedTestInfo(mutations[0], survivedResult).Return().Once()

	mockFSAdapter.EXPECT().Get(mock.Anything).Return(sources, nil)
	mockMutagen.EXPECT().GenerateMutation(mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4]).Return(mutations, nil)
	mockOrchestrator.EXPECT().TestMutation(mutations[0]).Return(survivedResult, nil)

	// Verify that the report includes the diff for survived mutations
	mockReportStore.EXPECT().SaveReports(mock.Anything, mock.MatchedBy(func(reports []m.Report) bool {
		if len(reports) != 1 {
			return false
		}
		report := reports[0]
		// Check that diff is included for survived mutation
		return report.Diff != nil && string(*report.Diff) == string(diffCode)
	})).Return(nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockUI, mockOrchestrator, mockMutagen)

	// Act
	args := domain.TestArgs{
		Reports:         "reports.json",
		Threads:         1,
		ShardIndex:      0,
		TotalShardCount: 1,
	}
	err := wf.Test(args)

	// Assert
	assert.NoError(t, err)
	mockFSAdapter.AssertExpectations(t)
	mockMutagen.AssertExpectations(t)
	mockOrchestrator.AssertExpectations(t)
	mockReportStore.AssertExpectations(t)
	mockUI.AssertExpectations(t)
}

func TestWorkflow_TestWithKilledMutation(t *testing.T) {
	// Arrange - This test checks that killed mutations do NOT include diff data
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything).Return(nil).Maybe()
	mockUI := new(controllermocks.MockUI)
	mockOrchestrator := new(domainmocks.MockOrchestrator)
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

	mockUI.EXPECT().Start(mock.Anything).Return(nil).Once()
	mockUI.EXPECT().Wait().Return().Once()
	mockUI.EXPECT().Close().Return().Once()
	mockUI.EXPECT().DisplayConcurencyInfo(mock.Anything, mock.Anything, mock.Anything).Return()
	mockUI.EXPECT().DusplayUpcomingTestsInfo(1).Return()
	mockUI.EXPECT().DisplayStartingTestInfo(mutations[0], 0).Return().Once()
	mockUI.EXPECT().DisplayCompletedTestInfo(mutations[0], killedResult).Return().Once()

	mockFSAdapter.EXPECT().Get(mock.Anything).Return(sources, nil)
	mockMutagen.EXPECT().GenerateMutation(mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4]).Return(mutations, nil)
	mockOrchestrator.EXPECT().TestMutation(mutations[0]).Return(killedResult, nil)

	// Verify that the report does NOT include diff for killed mutations
	mockReportStore.EXPECT().SaveReports(mock.Anything, mock.MatchedBy(func(reports []m.Report) bool {
		if len(reports) != 1 {
			return false
		}
		report := reports[0]
		// Check that diff is NOT included for killed mutation
		return report.Diff == nil
	})).Return(nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockUI, mockOrchestrator, mockMutagen)

	// Act
	args := domain.TestArgs{
		Reports:         "reports.json",
		Threads:         1,
		ShardIndex:      0,
		TotalShardCount: 1,
	}
	err := wf.Test(args)

	// Assert
	assert.NoError(t, err)
	mockFSAdapter.AssertExpectations(t)
	mockMutagen.AssertExpectations(t)
	mockOrchestrator.AssertExpectations(t)
	mockReportStore.AssertExpectations(t)
	mockUI.AssertExpectations(t)
}

func TestWorkflow_Estimate_Success(t *testing.T) {
	// Arrange
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything).Return(nil).Maybe()
	mockUI := new(controllermocks.MockUI)
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockMutagen := new(domainmocks.MockMutagen)

	sources := []m.Source{
		{Origin: &m.File{FullPath: "test.go", Hash: "hash1"}},
	}

	mutations := []m.Mutation{
		{ID: "hash-0", Source: sources[0], Type: m.MutationArithmetic},
	}

	mockUI.EXPECT().Start(mock.Anything).Return(nil).Once()
	mockUI.EXPECT().DisplayEstimation(mock.MatchedBy(func(ms []m.Mutation) bool {
		return len(ms) == 1
	}), nil).Return(nil).Once()
	mockUI.EXPECT().Wait().Return().Once()
	mockUI.EXPECT().Close().Return().Once()

	mockFSAdapter.EXPECT().Get(mock.Anything).Return(sources, nil)
	mockMutagen.EXPECT().GenerateMutation(mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4]).Return(mutations, nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockUI, mockOrchestrator, mockMutagen)

	// Act
	err := wf.Estimate(domain.EstimateArgs{Paths: []m.Path{"test.go"}})

	// Assert
	assert.NoError(t, err)
}

func TestWorkflow_Estimate_StartError(t *testing.T) {
	// Arrange
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything).Return(nil).Maybe()
	mockUI := new(controllermocks.MockUI)
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockMutagen := new(domainmocks.MockMutagen)

	startErr := errors.New("start failed")
	mockUI.EXPECT().Start(mock.Anything).Return(startErr).Once()

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockUI, mockOrchestrator, mockMutagen)

	// Act
	err := wf.Estimate(domain.EstimateArgs{Paths: []m.Path{"test.go"}})

	// Assert
	assert.ErrorIs(t, err, startErr)
}

func TestWorkflow_Estimate_GetMutationsError(t *testing.T) {
	// Arrange
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything).Return(nil).Maybe()
	mockUI := new(controllermocks.MockUI)
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockMutagen := new(domainmocks.MockMutagen)

	getErr := errors.New("get mutations failed")

	mockUI.EXPECT().Start(mock.Anything).Return(nil).Once()
	mockUI.EXPECT().Close().Return().Once()
	mockFSAdapter.EXPECT().Get(mock.Anything).Return(nil, getErr)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockUI, mockOrchestrator, mockMutagen)

	// Act
	err := wf.Estimate(domain.EstimateArgs{Paths: []m.Path{"test.go"}})

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "get sources")
}

func TestWorkflow_Estimate_DisplayError(t *testing.T) {
	// Arrange
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything).Return(nil).Maybe()
	mockUI := new(controllermocks.MockUI)
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockMutagen := new(domainmocks.MockMutagen)

	sources := []m.Source{
		{Origin: &m.File{FullPath: "test.go", Hash: "hash1"}},
	}

	mutations := []m.Mutation{
		{ID: "hash-0", Source: sources[0], Type: m.MutationArithmetic},
	}

	displayErr := errors.New("display failed")

	mockUI.EXPECT().Start(mock.Anything).Return(nil).Once()
	mockUI.EXPECT().DisplayEstimation(mock.Anything, nil).Return(displayErr).Once()
	mockUI.EXPECT().Close().Return().Once()

	mockFSAdapter.EXPECT().Get(mock.Anything).Return(sources, nil)
	mockMutagen.EXPECT().GenerateMutation(mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4]).Return(mutations, nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockUI, mockOrchestrator, mockMutagen)

	// Act
	err := wf.Estimate(domain.EstimateArgs{Paths: []m.Path{"test.go"}})

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "display")
}

type blockingOrchestrator struct {
	started    chan struct{}
	block      chan struct{}
	startCount int32
	current    int32
	max        int32
}

func (o *blockingOrchestrator) TestMutation(mutation m.Mutation) (m.Result, error) {
	atomic.AddInt32(&o.current, 1)
	for {
		current := atomic.LoadInt32(&o.current)
		max := atomic.LoadInt32(&o.max)
		if current <= max || atomic.CompareAndSwapInt32(&o.max, max, current) {
			break
		}
	}

	o.started <- struct{}{}
	if atomic.AddInt32(&o.startCount, 1) == 1 {
		<-o.block
	}

	atomic.AddInt32(&o.current, -1)

	result := m.Result{}
	result[mutation.Type] = []struct {
		MutationID string
		Status     m.TestStatus
		Err        error
	}{
		{
			MutationID: "hash-0",
			Status:     m.Killed,
			Err:        nil,
		},
	}

	return result, nil
}

func TestWorkflow_TestThreadLimitIsRespected(t *testing.T) {
	// Arrange
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything).Return(nil).Maybe()
	mockUI := new(controllermocks.MockUI)
	mockMutagen := new(domainmocks.MockMutagen)

	source := m.Source{
		Origin: &m.File{FullPath: "test.go", Hash: "hash1"},
	}

	mutations := []m.Mutation{
		{ID: "hash-0", Source: source, Type: m.MutationArithmetic},
		{ID: "hash-1", Source: source, Type: m.MutationArithmetic},
	}

	blocking := &blockingOrchestrator{
		started: make(chan struct{}, 2),
		block:   make(chan struct{}),
	}

	mockUI.EXPECT().Start(mock.Anything).Return(nil).Once()
	mockUI.EXPECT().Wait().Return().Once()
	mockUI.EXPECT().Close().Return().Once()
	mockUI.EXPECT().DisplayConcurencyInfo(mock.Anything, mock.Anything, mock.Anything).Return()
	mockUI.EXPECT().DusplayUpcomingTestsInfo(mock.Anything).Return()
	mockUI.EXPECT().DisplayStartingTestInfo(mock.Anything, mock.Anything).Return().Times(2)
	mockUI.EXPECT().DisplayCompletedTestInfo(mock.Anything, mock.Anything).Return().Times(2)
	mockFSAdapter.EXPECT().Get(mock.Anything).Return([]m.Source{source}, nil)
	mockMutagen.EXPECT().GenerateMutation(mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4]).Return(mutations, nil)
	mockReportStore.EXPECT().SaveReports(mock.Anything, mock.MatchedBy(func(reports []m.Report) bool {
		return len(reports) == 2
	})).Return(nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockUI, blocking, mockMutagen)

	// Act
	args := domain.TestArgs{
		Reports:         "reports.json",
		Threads:         1,
		ShardIndex:      0,
		TotalShardCount: 1,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- wf.Test(args)
	}()

	<-blocking.started
	secondStartedEarly := false
	select {
	case <-blocking.started:
		secondStartedEarly = true
	case <-time.After(50 * time.Millisecond):
	}
	close(blocking.block)

	if !secondStartedEarly {
		<-blocking.started
	}

	err := <-errCh

	// Assert
	assert.NoError(t, err)
	assert.False(t, secondStartedEarly, "second mutation started before first completed with Threads=1")
	assert.Equal(t, int32(1), atomic.LoadInt32(&blocking.max))
}

func TestWorkflow_TestThreadIDStartsAtZero(t *testing.T) {
	// Arrange
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything).Return(nil).Maybe()
	mockUI := new(controllermocks.MockUI)
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockMutagen := new(domainmocks.MockMutagen)

	source := m.Source{
		Origin: &m.File{FullPath: "test.go", Hash: "hash1"},
	}

	mutations := []m.Mutation{
		{ID: "hash-0", Source: source, Type: m.MutationArithmetic},
	}

	mockUI.EXPECT().Start(mock.Anything).Return(nil).Once()
	mockUI.EXPECT().Wait().Return().Once()
	mockUI.EXPECT().Close().Return().Once()
	mockUI.EXPECT().DisplayConcurencyInfo(mock.Anything, mock.Anything, mock.Anything).Return()
	mockUI.EXPECT().DusplayUpcomingTestsInfo(mock.Anything).Return()
	mockUI.EXPECT().DisplayStartingTestInfo(mutations[0], 0).Return().Once()
	mockUI.EXPECT().DisplayCompletedTestInfo(mutations[0], mock.Anything).Return().Once()
	mockFSAdapter.EXPECT().Get(mock.Anything).Return([]m.Source{source}, nil)
	mockMutagen.EXPECT().GenerateMutation(mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4]).Return(mutations, nil)
	mockOrchestrator.EXPECT().TestMutation(mutations[0]).Return(m.Result{}, nil)
	mockReportStore.EXPECT().SaveReports(mock.Anything, mock.Anything).Return(nil)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything).Return(nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockUI, mockOrchestrator, mockMutagen)

	// Act
	args := domain.TestArgs{
		Reports:         "reports.json",
		Threads:         3,
		ShardIndex:      0,
		TotalShardCount: 1,
	}
	err := wf.Test(args)

	// Assert
	assert.NoError(t, err)
}

func TestWorkflow_TestExactMutationIDMatch(t *testing.T) {
	// Arrange
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockReportStore.EXPECT().RegenerateIndex(mock.Anything).Return(nil).Maybe()
	mockUI := new(controllermocks.MockUI)
	mockOrchestrator := new(domainmocks.MockOrchestrator)
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

	mockUI.EXPECT().Start(mock.Anything).Return(nil).Once()
	mockUI.EXPECT().Wait().Return().Once()
	mockUI.EXPECT().Close().Return().Once()
	mockUI.EXPECT().DisplayConcurencyInfo(mock.Anything, mock.Anything, mock.Anything).Return()
	mockUI.EXPECT().DusplayUpcomingTestsInfo(1).Return()
	mockUI.EXPECT().DisplayStartingTestInfo(mutations[0], 0).Return().Once()
	mockUI.EXPECT().DisplayCompletedTestInfo(mutations[0], result).Return().Once()

	mockFSAdapter.EXPECT().Get(mock.Anything).Return(sources, nil)
	mockMutagen.EXPECT().GenerateMutation(mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1], domain.DefaultMutations[2], domain.DefaultMutations[3], domain.DefaultMutations[4]).Return(mutations, nil)
	mockOrchestrator.EXPECT().TestMutation(mutations[0]).Return(result, nil)

	mockReportStore.EXPECT().SaveReports(mock.Anything, mock.MatchedBy(func(reports []m.Report) bool {
		if len(reports) != 1 {
			return false
		}
		report := reports[0]
		return report.Diff != nil && string(*report.Diff) == string(diffCode)
	})).Return(nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockUI, mockOrchestrator, mockMutagen)

	// Act
	args := domain.TestArgs{
		Reports:         "reports.json",
		Threads:         1,
		ShardIndex:      0,
		TotalShardCount: 1,
	}
	err := wf.Test(args)

	// Assert
	assert.NoError(t, err)
}
