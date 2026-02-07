package domain_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	adaptermocks "gooze.dev/pkg/gooze/internal/adapter/mocks"
	controllermocks "gooze.dev/pkg/gooze/internal/controller/mocks"
	domain "gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
	m "gooze.dev/pkg/gooze/internal/model"
)

func TestWorkflowPipeline_Estimate_Success(t *testing.T) {
	// Arrange
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockUI := new(controllermocks.MockUI)
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockMutagen := new(domainmocks.MockMutagen)

	sources := []m.Source{
		{Origin: &m.File{FullPath: "test.go", Hash: "hash1"}},
	}

	mutations := []m.Mutation{
		{ID: "hash-0", Source: sources[0], Type: m.MutationArithmetic},
		{ID: "hash-1", Source: sources[0], Type: m.MutationBoolean},
	}

	threads := 1

	// Setup mock expectations
	mockUI.EXPECT().Start(mock.Anything).Return(nil).Once()
	mockUI.EXPECT().Wait().Return().Once()
	mockUI.EXPECT().Close().Return().Once()

	// Mock GetChannel to return sources
	sourceChan := make(chan m.Source, threads)
	sourceErrChan := make(chan error, 1)
	close(sourceErrChan)
	mockFSAdapter.EXPECT().GetChannel(mock.Anything, mock.Anything, threads, mock.Anything).
		Run(func(ctx context.Context, paths []m.Path, threads int, exclude ...string) {
			go func() {
				defer close(sourceChan)
				for _, src := range sources {
					select {
					case <-ctx.Done():
						return
					case sourceChan <- src:
					}
				}
			}()
		}).
		Return((<-chan m.Source)(sourceChan), (<-chan error)(sourceErrChan))

	// Mock GenerateMutation to return mutations for each source
	mockMutagen.EXPECT().GenerateMutation(mock.Anything,
		domain.DefaultMutations[0],
		domain.DefaultMutations[1],
		domain.DefaultMutations[2],
		domain.DefaultMutations[3],
		domain.DefaultMutations[4],
		domain.DefaultMutations[5]).Return(mutations, nil).Once()

	// Mock DisplayEstimation to capture the collected mutations
	mockUI.EXPECT().DisplayEstimation(mock.MatchedBy(func(muts []m.Mutation) bool {
		return len(muts) == len(mutations)
	}), mock.Anything).Return(nil).Once()

	wf := domain.NewWorkflowPipeline(mockFSAdapter, mockReportStore, mockUI, mockOrchestrator, mockMutagen)

	// Act
	err := wf.Estimate(domain.EstimateArgs{
		Paths: []m.Path{"test.go"},
	})

	// Assert
	assert.NoError(t, err)
	mockFSAdapter.AssertExpectations(t)
	mockMutagen.AssertExpectations(t)
	mockUI.AssertExpectations(t)
}

func TestWorkflowPipeline_Estimate_StartError(t *testing.T) {
	// Arrange
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockUI := new(controllermocks.MockUI)
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockMutagen := new(domainmocks.MockMutagen)

	startErr := errors.New("start failed")
	mockUI.EXPECT().Start(mock.Anything).Return(startErr).Once()

	wf := domain.NewWorkflowPipeline(mockFSAdapter, mockReportStore, mockUI, mockOrchestrator, mockMutagen)

	// Act
	err := wf.Estimate(domain.EstimateArgs{Paths: []m.Path{"test.go"}})

	// Assert
	assert.ErrorIs(t, err, startErr)
	mockUI.AssertExpectations(t)
}

func TestWorkflowPipeline_Estimate_GetSourcesError(t *testing.T) {
	// Arrange
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockUI := new(controllermocks.MockUI)
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockMutagen := new(domainmocks.MockMutagen)

	threads := 1
	getErr := errors.New("get sources failed")

	mockUI.EXPECT().Start(mock.Anything).Return(nil).Once()
	mockUI.EXPECT().Close().Return().Once()

	// Mock GetChannel to return error
	sourceChan := make(chan m.Source, threads)
	close(sourceChan)
	sourceErrChan := make(chan error, 1)
	mockFSAdapter.EXPECT().GetChannel(mock.Anything, mock.Anything, threads, mock.Anything).
		Run(func(ctx context.Context, paths []m.Path, threads int, exclude ...string) {
			go func() {
				defer close(sourceErrChan)
				sourceErrChan <- getErr
			}()
		}).
		Return((<-chan m.Source)(sourceChan), (<-chan error)(sourceErrChan))

	wf := domain.NewWorkflowPipeline(mockFSAdapter, mockReportStore, mockUI, mockOrchestrator, mockMutagen)

	// Act
	err := wf.Estimate(domain.EstimateArgs{Paths: []m.Path{"test.go"}})

	// Assert
	assert.Error(t, err)
	assert.ErrorIs(t, err, getErr)
	mockFSAdapter.AssertExpectations(t)
	mockUI.AssertExpectations(t)
}

func TestWorkflowPipeline_Estimate_GenerateMutationsError(t *testing.T) {
	// Arrange
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockUI := new(controllermocks.MockUI)
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockMutagen := new(domainmocks.MockMutagen)

	sources := []m.Source{
		{Origin: &m.File{FullPath: "test.go", Hash: "hash1"}},
	}

	threads := 1
	genErr := errors.New("generate mutations failed")

	mockUI.EXPECT().Start(mock.Anything).Return(nil).Once()
	mockUI.EXPECT().Close().Return().Once()

	// Mock GetChannel to return sources
	sourceChan := make(chan m.Source, threads)
	sourceErrChan := make(chan error, 1)
	close(sourceErrChan)
	mockFSAdapter.EXPECT().GetChannel(mock.Anything, mock.Anything, threads, mock.Anything).
		Run(func(ctx context.Context, paths []m.Path, threads int, exclude ...string) {
			go func() {
				defer close(sourceChan)
				for _, src := range sources {
					sourceChan <- src
				}
			}()
		}).
		Return((<-chan m.Source)(sourceChan), (<-chan error)(sourceErrChan))

	// Mock GenerateMutation to return error
	mockMutagen.EXPECT().GenerateMutation(mock.Anything,
		domain.DefaultMutations[0],
		domain.DefaultMutations[1],
		domain.DefaultMutations[2],
		domain.DefaultMutations[3],
		domain.DefaultMutations[4],
		domain.DefaultMutations[5]).Return(nil, genErr).Once()

	wf := domain.NewWorkflowPipeline(mockFSAdapter, mockReportStore, mockUI, mockOrchestrator, mockMutagen)

	// Act
	err := wf.Estimate(domain.EstimateArgs{Paths: []m.Path{"test.go"}})

	// Assert
	assert.Error(t, err)
	assert.ErrorIs(t, err, genErr)
	mockFSAdapter.AssertExpectations(t)
	mockMutagen.AssertExpectations(t)
	mockUI.AssertExpectations(t)
}

func TestWorkflowPipeline_Estimate_DisplayError(t *testing.T) {
	// Arrange
	mockFSAdapter := new(adaptermocks.MockSourceFSAdapter)
	mockReportStore := new(adaptermocks.MockReportStore)
	mockUI := new(controllermocks.MockUI)
	mockOrchestrator := new(domainmocks.MockOrchestrator)
	mockMutagen := new(domainmocks.MockMutagen)

	sources := []m.Source{
		{Origin: &m.File{FullPath: "test.go", Hash: "hash1"}},
	}

	mutations := []m.Mutation{
		{ID: "hash-0", Source: sources[0], Type: m.MutationArithmetic},
	}

	threads := 1
	displayErr := errors.New("display failed")

	mockUI.EXPECT().Start(mock.Anything).Return(nil).Once()
	mockUI.EXPECT().DisplayEstimation(mock.Anything, nil).Return(displayErr).Once()
	mockUI.EXPECT().Close().Return().Once()

	// Mock GetChannel to return sources
	sourceChan := make(chan m.Source, threads)
	sourceErrChan := make(chan error, 1)
	close(sourceErrChan)
	mockFSAdapter.EXPECT().GetChannel(mock.Anything, mock.Anything, threads, mock.Anything).
		Run(func(ctx context.Context, paths []m.Path, threads int, exclude ...string) {
			go func() {
				defer close(sourceChan)
				for _, src := range sources {
					sourceChan <- src
				}
			}()
		}).
		Return((<-chan m.Source)(sourceChan), (<-chan error)(sourceErrChan))

	// Mock GenerateMutation to return mutations
	mockMutagen.EXPECT().GenerateMutation(mock.Anything,
		domain.DefaultMutations[0],
		domain.DefaultMutations[1],
		domain.DefaultMutations[2],
		domain.DefaultMutations[3],
		domain.DefaultMutations[4],
		domain.DefaultMutations[5]).Return(mutations, nil).Once()

	wf := domain.NewWorkflowPipeline(mockFSAdapter, mockReportStore, mockUI, mockOrchestrator, mockMutagen)

	// Act
	err := wf.Estimate(domain.EstimateArgs{Paths: []m.Path{"test.go"}})

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "display")
	mockFSAdapter.AssertExpectations(t)
	mockMutagen.AssertExpectations(t)
	mockUI.AssertExpectations(t)
}
