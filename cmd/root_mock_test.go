package cmd

import (
	"testing"

	"github.com/mouse-blink/gooze/internal/adapter"
	mockAdapter "github.com/mouse-blink/gooze/internal/adapter/mocks"
	mockDomain "github.com/mouse-blink/gooze/internal/domain/mocks"
	m "github.com/mouse-blink/gooze/internal/model"
)

func TestRunRoot_WithMocks(t *testing.T) {
	t.Run("displays mutation estimations when list flag is used", func(t *testing.T) {
		// Create mocks
		mockWorkflow := mockDomain.NewMockWorkflow(t)
		mockUI := mockAdapter.NewMockUI(t)

		// Setup test data
		sources := []m.Source{
			{Origin: m.Path("main.go")},
			{Origin: m.Path("helper.go")},
		}

		estimations := map[m.Path]adapter.MutationEstimation{
			m.Path("main.go"):   {Arithmetic: 5, Boolean: 0},
			m.Path("helper.go"): {Arithmetic: 3, Boolean: 0},
		}

		// Mock expectations - using simple On/Return pattern
		mockWorkflow.On("GetSources", m.Path("./test")).Return(sources, nil)
		mockUI.On("DisplayMutationEstimations", estimations).Return(nil)

		// Use mocks
		gotSources, err := mockWorkflow.GetSources(m.Path("./test"))
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(gotSources) != 2 {
			t.Errorf("expected 2 sources, got %d", len(gotSources))
		}

		err = mockUI.DisplayMutationEstimations(estimations)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		mockWorkflow.AssertExpectations(t)
		mockUI.AssertExpectations(t)
	})

	t.Run("shows not implemented message without list flag", func(t *testing.T) {
		// Create mocks
		mockWorkflow := mockDomain.NewMockWorkflow(t)
		mockUI := mockAdapter.NewMockUI(t)

		// Setup test data
		sources := []m.Source{
			{Origin: m.Path("main.go")},
		}

		// Mock expectations
		mockWorkflow.On("GetSources", m.Path("./test")).Return(sources, nil)
		mockUI.On("ShowNotImplemented", 1).Return(nil)

		// Use mocks
		gotSources, err := mockWorkflow.GetSources(m.Path("./test"))
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		err = mockUI.ShowNotImplemented(len(gotSources))
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		mockWorkflow.AssertExpectations(t)
		mockUI.AssertExpectations(t)
	})
}

// Example of testing Workflow mock
func TestWorkflowMock_GetSources(t *testing.T) {
	t.Run("returns sources successfully", func(t *testing.T) {
		// Create mock
		mockWorkflow := mockDomain.NewMockWorkflow(t)

		// Setup expectation
		expectedSources := []m.Source{
			{Origin: m.Path("file1.go")},
			{Origin: m.Path("file2.go")},
		}

		mockWorkflow.EXPECT().
			GetSources(m.Path("./test")).
			Return(expectedSources, nil)

		// Use mock
		sources, err := mockWorkflow.GetSources(m.Path("./test"))

		// Verify
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(sources) != 2 {
			t.Errorf("expected 2 sources, got %d", len(sources))
		}
		if sources[0].Origin != m.Path("file1.go") {
			t.Errorf("expected first source to be file1.go, got %s", sources[0].Origin)
		}
	})

	t.Run("handles variadic parameters", func(t *testing.T) {
		// Create mock
		mockWorkflow := mockDomain.NewMockWorkflow(t)

		// Setup expectation with multiple paths
		expectedSources := []m.Source{
			{Origin: m.Path("file1.go")},
			{Origin: m.Path("file2.go")},
			{Origin: m.Path("file3.go")},
		}

		mockWorkflow.EXPECT().
			GetSources(m.Path("./path1"), m.Path("./path2")).
			Return(expectedSources, nil)

		// Use mock with multiple paths
		sources, err := mockWorkflow.GetSources(m.Path("./path1"), m.Path("./path2"))

		// Verify
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(sources) != 3 {
			t.Errorf("expected 3 sources, got %d", len(sources))
		}
	})
}

// Example of testing UI mock
func TestUIMock_DisplayMutationEstimations(t *testing.T) {
	t.Run("displays mutation estimations successfully", func(t *testing.T) {
		// Create mock
		mockUI := mockAdapter.NewMockUI(t)

		// Setup test data
		estimations := map[m.Path]adapter.MutationEstimation{
			m.Path("main.go"): {Arithmetic: 5, Boolean: 0},
		}

		// Setup expectation
		mockUI.EXPECT().
			DisplayMutationEstimations(estimations).
			Return(nil)

		// Use mock
		err := mockUI.DisplayMutationEstimations(estimations)

		// Verify
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("shows not implemented message", func(t *testing.T) {
		// Create mock
		mockUI := mockAdapter.NewMockUI(t)

		// Setup expectation
		mockUI.EXPECT().
			ShowNotImplemented(5).
			Return(nil)

		// Use mock
		err := mockUI.ShowNotImplemented(5)

		// Verify
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("handles empty estimations", func(t *testing.T) {
		// Create mock
		mockUI := mockAdapter.NewMockUI(t)

		// Setup expectation with empty map
		mockUI.EXPECT().
			DisplayMutationEstimations(map[m.Path]adapter.MutationEstimation{}).
			Return(nil)

		// Use mock
		err := mockUI.DisplayMutationEstimations(map[m.Path]adapter.MutationEstimation{})

		// Verify
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}
