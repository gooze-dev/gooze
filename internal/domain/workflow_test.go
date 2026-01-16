package domain_test

import (
	"errors"
	"testing"

	adaptermocks "github.com/mouse-blink/gooze/internal/adapter/mocks"
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
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockMutagen := new(domainmocks.MockMutagen)

	sources := []m.Source{
		{
			Origin: &m.File{Path: "test.go", Hash: "hash1"},
			Test:   &m.File{Path: "test_test.go", Hash: "test_hash1"},
		},
	}

	mutations := []m.Mutation{
		{ID: 1, Source: sources[0], Type: m.MutationArithmetic},
	}

	mockFSAdapter.EXPECT().Get(mock.Anything).Return(sources, nil)
	mockMutagen.EXPECT().GenerateMutation(mock.Anything, mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1]).Return(mutations, nil)
	mockOrchestrator.EXPECT().TestMutation(mock.Anything).Return(m.Result{}, nil)
	mockReportStore.EXPECT().SaveReports(mock.Anything, mock.Anything).Return(nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockOrchestrator, mockMutagen)

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
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockMutagen := new(domainmocks.MockMutagen)

	testErr := errors.New("failed to get sources")
	mockFSAdapter.EXPECT().Get(mock.Anything).Return(nil, testErr)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockOrchestrator, mockMutagen)

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
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockMutagen := new(domainmocks.MockMutagen)

	sources := []m.Source{
		{Origin: &m.File{Path: "test.go", Hash: "hash1"}},
	}

	testErr := errors.New("failed to generate mutations")
	mockFSAdapter.EXPECT().Get(mock.Anything).Return(sources, nil)
	mockMutagen.EXPECT().GenerateMutation(mock.Anything, mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1]).Return(nil, testErr)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockOrchestrator, mockMutagen)

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
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockMutagen := new(domainmocks.MockMutagen)

	sources := []m.Source{
		{Origin: &m.File{Path: "test.go", Hash: "hash1"}},
	}

	mutations := []m.Mutation{
		{ID: 1, Source: sources[0]},
	}

	testErr := errors.New("failed to test mutation")
	mockFSAdapter.EXPECT().Get(mock.Anything).Return(sources, nil)
	mockMutagen.EXPECT().GenerateMutation(mock.Anything, mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1]).Return(mutations, nil)
	mockOrchestrator.EXPECT().TestMutation(mock.Anything).Return(nil, testErr)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockOrchestrator, mockMutagen)

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
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockMutagen := new(domainmocks.MockMutagen)

	sources := []m.Source{
		{Origin: &m.File{Path: "test.go", Hash: "hash1"}},
	}

	mutations := []m.Mutation{
		{ID: 1, Source: sources[0]},
	}

	mockFSAdapter.EXPECT().Get(mock.Anything).Return(sources, nil)
	mockMutagen.EXPECT().GenerateMutation(mock.Anything, mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1]).Return(mutations, nil)
	mockOrchestrator.EXPECT().TestMutation(mock.Anything).Return(m.Result{}, nil)

	saveErr := errors.New("failed to save reports")
	mockReportStore.EXPECT().SaveReports(mock.Anything, mock.Anything).Return(saveErr)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockOrchestrator, mockMutagen)

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
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockMutagen := new(domainmocks.MockMutagen)

	sources := []m.Source{
		{Origin: &m.File{Path: "test.go", Hash: "hash1"}},
	}

	// No mutations generated
	mockFSAdapter.EXPECT().Get(mock.Anything).Return(sources, nil)
	mockMutagen.EXPECT().GenerateMutation(mock.Anything, mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1]).Return([]m.Mutation{}, nil)
	mockReportStore.EXPECT().SaveReports(mock.Anything, mock.MatchedBy(func(reports []m.Report) bool {
		return len(reports) == 0
	})).Return(nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockOrchestrator, mockMutagen)

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
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockMutagen := new(domainmocks.MockMutagen)

	source := m.Source{
		Origin: &m.File{Path: "test.go", Hash: "hash1"},
	}

	mutations := []m.Mutation{
		{ID: 0, Source: source},
		{ID: 1, Source: source},
		{ID: 2, Source: source},
	}

	mockFSAdapter.EXPECT().Get(mock.Anything).Return([]m.Source{source}, nil)
	mockMutagen.EXPECT().GenerateMutation(mock.Anything, mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1]).Return(mutations, nil)
	mockOrchestrator.EXPECT().TestMutation(mock.Anything).Return(m.Result{}, nil).Times(3)
	mockReportStore.EXPECT().SaveReports(mock.Anything, mock.MatchedBy(func(reports []m.Report) bool {
		return len(reports) == 3
	})).Return(nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockOrchestrator, mockMutagen)

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
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockMutagen := new(domainmocks.MockMutagen)

	source := m.Source{
		Origin: &m.File{Path: "test.go", Hash: "hash1"},
	}

	// 6 mutations total
	mutations := []m.Mutation{
		{ID: 0, Source: source},
		{ID: 1, Source: source},
		{ID: 2, Source: source},
		{ID: 3, Source: source},
		{ID: 4, Source: source},
		{ID: 5, Source: source},
	}

	mockFSAdapter.EXPECT().Get(mock.Anything).Return([]m.Source{source}, nil)
	mockMutagen.EXPECT().GenerateMutation(mock.Anything, mock.Anything, domain.DefaultMutations[0], domain.DefaultMutations[1]).Return(mutations, nil)
	// Only 2 mutations should be tested (IDs 0 and 3, since shardIndex=0, totalShards=3)
	mockOrchestrator.EXPECT().TestMutation(mock.Anything).Return(m.Result{}, nil).Times(2)
	mockReportStore.EXPECT().SaveReports(mock.Anything, mock.MatchedBy(func(reports []m.Report) bool {
		return len(reports) == 2
	})).Return(nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockOrchestrator, mockMutagen)

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

func TestWorkflow_Test_MultipleSources(t *testing.T) {
	// Arrange
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockMutagen := new(domainmocks.MockMutagen)

	source1 := m.Source{
		Origin: &m.File{Path: "file1.go", Hash: "hash1"},
	}
	source2 := m.Source{
		Origin: &m.File{Path: "file2.go", Hash: "hash2"},
	}

	mutations1 := []m.Mutation{
		{ID: 0, Source: source1},
		{ID: 1, Source: source1},
	}
	mutations2 := []m.Mutation{
		{ID: 2, Source: source2},
	}

	mockFSAdapter.EXPECT().Get(mock.Anything).Return([]m.Source{source1, source2}, nil)
	mockMutagen.EXPECT().GenerateMutation(source1, 0, domain.DefaultMutations[0], domain.DefaultMutations[1]).Return(mutations1, nil)
	mockMutagen.EXPECT().GenerateMutation(source2, 2, domain.DefaultMutations[0], domain.DefaultMutations[1]).Return(mutations2, nil)
	mockOrchestrator.EXPECT().TestMutation(mock.Anything).Return(m.Result{}, nil).Times(3)
	mockReportStore.EXPECT().SaveReports(mock.Anything, mock.MatchedBy(func(reports []m.Report) bool {
		return len(reports) == 3
	})).Return(nil)

	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockOrchestrator, mockMutagen)

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
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockMutagen := new(domainmocks.MockMutagen)

	// Act
	wf := domain.NewWorkflow(mockFSAdapter, mockReportStore, mockOrchestrator, mockMutagen)

	// Assert
	require.NotNil(t, wf)
	assert.Implements(t, (*domain.Workflow)(nil), wf)
}
