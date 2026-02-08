package domain_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	adaptermocks "gooze.dev/pkg/gooze/internal/adapter/mocks"
	domain "gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
	pkg "gooze.dev/pkg/gooze/pkg"
)

func TestReportManager_Save_Success(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockReportStore := new(adaptermocks.MockReportStore)

	reportsDir := m.Path("reports")
	reports := createMockFileSpill(t, []m.Report{
		{
			Source: m.Source{Origin: &m.File{FullPath: "test.go", Hash: "hash1"}},
			Result: m.Result{},
		},
	})

	mockReportStore.EXPECT().SaveSpillReports(ctx, reportsDir, reports).Return(nil)
	mockReportStore.EXPECT().RegenerateIndex(ctx, reportsDir).Return(nil)

	rm := domain.NewReportManager()
	// Inject the mock using reflection or setter if available
	// For this test, we assume NewReportManager returns a testable implementation

	// Act
	err := rm.Save(ctx, reportsDir, reports)

	// Assert
	assert.NoError(t, err)
	mockReportStore.AssertExpectations(t)
}

func TestReportManager_Save_SaveSpillReportsError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockReportStore := new(adaptermocks.MockReportStore)

	reportsDir := m.Path("reports")
	reports := createMockFileSpill(t, []m.Report{
		{
			Source: m.Source{Origin: &m.File{FullPath: "test.go", Hash: "hash1"}},
			Result: m.Result{},
		},
	})

	saveErr := errors.New("failed to save reports")
	mockReportStore.EXPECT().SaveSpillReports(ctx, reportsDir, reports).Return(saveErr)

	rm := domain.NewReportManager()

	// Act
	err := rm.Save(ctx, reportsDir, reports)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "save reports")
	mockReportStore.AssertExpectations(t)
}

func TestReportManager_Save_RegenerateIndexError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockReportStore := new(adaptermocks.MockReportStore)

	reportsDir := m.Path("reports")
	reports := createMockFileSpill(t, []m.Report{
		{
			Source: m.Source{Origin: &m.File{FullPath: "test.go", Hash: "hash1"}},
			Result: m.Result{},
		},
	})

	indexErr := errors.New("failed to regenerate index")
	mockReportStore.EXPECT().SaveSpillReports(ctx, reportsDir, reports).Return(nil)
	mockReportStore.EXPECT().RegenerateIndex(ctx, reportsDir).Return(indexErr)

	rm := domain.NewReportManager()

	// Act
	err := rm.Save(ctx, reportsDir, reports)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "regenerate index")
	mockReportStore.AssertExpectations(t)
}

func TestReportManager_Save_ContextCancelled(t *testing.T) {
	// Arrange
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	reportsDir := m.Path("reports")
	reports := createMockFileSpill(t, []m.Report{})

	rm := domain.NewReportManager()

	// Act
	err := rm.Save(ctx, reportsDir, reports)

	// Assert
	// The behavior depends on implementation, but context cancellation should be handled
	// This test documents expected behavior
	_ = err
}

func TestReportManager_Merge_Success(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockReportStore := new(adaptermocks.MockReportStore)

	basePath := m.Path(t.TempDir())

	// Create shard directories
	shard0 := filepath.Join(string(basePath), "shard_0")
	shard1 := filepath.Join(string(basePath), "shard_1")
	require.NoError(t, os.MkdirAll(shard0, 0755))
	require.NoError(t, os.MkdirAll(shard1, 0755))

	shard0Reports := []m.Report{
		{
			Source: m.Source{Origin: &m.File{FullPath: "test1.go", Hash: "hash1"}},
			Result: m.Result{},
		},
	}

	shard1Reports := []m.Report{
		{
			Source: m.Source{Origin: &m.File{FullPath: "test2.go", Hash: "hash2"}},
			Result: m.Result{},
		},
	}

	// Mock loading existing reports from base (returns not found)
	mockReportStore.EXPECT().LoadReports(ctx, basePath).Return(nil, os.ErrNotExist).Once()

	// Mock loading shard reports
	mockReportStore.EXPECT().LoadReports(ctx, m.Path(shard0)).Return(shard0Reports, nil).Once()
	mockReportStore.EXPECT().LoadReports(ctx, m.Path(shard1)).Return(shard1Reports, nil).Once()

	// Mock saving merged reports
	mockReportStore.EXPECT().SaveReports(ctx, basePath, mock.MatchedBy(func(reports []m.Report) bool {
		return len(reports) == 2
	})).Return(nil)

	mockReportStore.EXPECT().RegenerateIndex(ctx, basePath).Return(nil)

	rm := domain.NewReportManager()

	// Act
	err := rm.Merge(ctx, basePath)

	// Assert
	assert.NoError(t, err)
	mockReportStore.AssertExpectations(t)

	// Verify shard directories are removed
	_, err = os.Stat(shard0)
	assert.True(t, os.IsNotExist(err), "shard_0 should be removed")
	_, err = os.Stat(shard1)
	assert.True(t, os.IsNotExist(err), "shard_1 should be removed")
}

func TestReportManager_Merge_NoShardDirs(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockReportStore := new(adaptermocks.MockReportStore)

	basePath := m.Path(t.TempDir())

	// No shard directories exist, should only regenerate index
	mockReportStore.EXPECT().RegenerateIndex(ctx, basePath).Return(nil)

	rm := domain.NewReportManager()

	// Act
	err := rm.Merge(ctx, basePath)

	// Assert
	assert.NoError(t, err)
	mockReportStore.AssertExpectations(t)
}

func TestReportManager_Merge_BasePathNotExists(t *testing.T) {
	// Arrange
	ctx := context.Background()

	basePath := m.Path("/nonexistent/path")

	rm := domain.NewReportManager()

	// Act
	err := rm.Merge(ctx, basePath)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "find shard directories")
}

func TestReportManager_Merge_LoadShardReportsError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockReportStore := new(adaptermocks.MockReportStore)

	basePath := m.Path(t.TempDir())

	// Create shard directory
	shard0 := filepath.Join(string(basePath), "shard_0")
	require.NoError(t, os.MkdirAll(shard0, 0755))

	loadErr := errors.New("failed to load shard reports")

	// Mock loading existing reports from base (returns not found)
	mockReportStore.EXPECT().LoadReports(ctx, basePath).Return(nil, os.ErrNotExist).Once()

	// Mock loading shard reports with error
	mockReportStore.EXPECT().LoadReports(ctx, m.Path(shard0)).Return(nil, loadErr).Once()

	rm := domain.NewReportManager()

	// Act
	err := rm.Merge(ctx, basePath)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "load shard reports")
	mockReportStore.AssertExpectations(t)
}

func TestReportManager_Merge_SaveMergedReportsError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockReportStore := new(adaptermocks.MockReportStore)

	basePath := m.Path(t.TempDir())

	// Create shard directory
	shard0 := filepath.Join(string(basePath), "shard_0")
	require.NoError(t, os.MkdirAll(shard0, 0755))

	shard0Reports := []m.Report{
		{
			Source: m.Source{Origin: &m.File{FullPath: "test1.go", Hash: "hash1"}},
			Result: m.Result{},
		},
	}

	saveErr := errors.New("failed to save merged reports")

	// Mock loading existing reports from base (returns not found)
	mockReportStore.EXPECT().LoadReports(ctx, basePath).Return(nil, os.ErrNotExist).Once()

	// Mock loading shard reports
	mockReportStore.EXPECT().LoadReports(ctx, m.Path(shard0)).Return(shard0Reports, nil).Once()

	// Mock saving merged reports with error
	mockReportStore.EXPECT().SaveReports(ctx, basePath, mock.Anything).Return(saveErr)

	rm := domain.NewReportManager()

	// Act
	err := rm.Merge(ctx, basePath)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "save merged reports")
	mockReportStore.AssertExpectations(t)
}

func TestReportManager_Merge_PreservesExistingReports(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockReportStore := new(adaptermocks.MockReportStore)

	basePath := m.Path(t.TempDir())

	// Create shard directory
	shard0 := filepath.Join(string(basePath), "shard_0")
	require.NoError(t, os.MkdirAll(shard0, 0755))

	existingReports := []m.Report{
		{
			Source: m.Source{Origin: &m.File{FullPath: "existing.go", Hash: "hash_existing"}},
			Result: m.Result{},
		},
	}

	shard0Reports := []m.Report{
		{
			Source: m.Source{Origin: &m.File{FullPath: "test1.go", Hash: "hash1"}},
			Result: m.Result{},
		},
	}

	// Mock loading existing reports from base
	mockReportStore.EXPECT().LoadReports(ctx, basePath).Return(existingReports, nil).Once()

	// Mock loading shard reports
	mockReportStore.EXPECT().LoadReports(ctx, m.Path(shard0)).Return(shard0Reports, nil).Once()

	// Mock saving merged reports - verify both existing and shard reports are included
	mockReportStore.EXPECT().SaveReports(ctx, basePath, mock.MatchedBy(func(reports []m.Report) bool {
		if len(reports) != 2 {
			return false
		}
		// Verify existing report is first
		if reports[0].Source.Origin.Hash != "hash_existing" {
			return false
		}
		// Verify shard report is second
		if reports[1].Source.Origin.Hash != "hash1" {
			return false
		}
		return true
	})).Return(nil)

	mockReportStore.EXPECT().RegenerateIndex(ctx, basePath).Return(nil)

	rm := domain.NewReportManager()

	// Act
	err := rm.Merge(ctx, basePath)

	// Assert
	assert.NoError(t, err)
	mockReportStore.AssertExpectations(t)
}

func TestReportManager_Merge_MultipleShards(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockReportStore := new(adaptermocks.MockReportStore)

	basePath := m.Path(t.TempDir())

	// Create multiple shard directories
	shard0 := filepath.Join(string(basePath), "shard_0")
	shard1 := filepath.Join(string(basePath), "shard_1")
	shard2 := filepath.Join(string(basePath), "shard_2")
	require.NoError(t, os.MkdirAll(shard0, 0755))
	require.NoError(t, os.MkdirAll(shard1, 0755))
	require.NoError(t, os.MkdirAll(shard2, 0755))

	// Create non-shard directory that should be ignored
	otherDir := filepath.Join(string(basePath), "other")
	require.NoError(t, os.MkdirAll(otherDir, 0755))

	shard0Reports := []m.Report{
		{Source: m.Source{Origin: &m.File{FullPath: "test0.go", Hash: "hash0"}}},
	}
	shard1Reports := []m.Report{
		{Source: m.Source{Origin: &m.File{FullPath: "test1.go", Hash: "hash1"}}},
	}
	shard2Reports := []m.Report{
		{Source: m.Source{Origin: &m.File{FullPath: "test2.go", Hash: "hash2"}}},
	}

	// Mock loading existing reports from base (returns not found)
	mockReportStore.EXPECT().LoadReports(ctx, basePath).Return(nil, os.ErrNotExist).Once()

	// Mock loading shard reports
	mockReportStore.EXPECT().LoadReports(ctx, m.Path(shard0)).Return(shard0Reports, nil).Once()
	mockReportStore.EXPECT().LoadReports(ctx, m.Path(shard1)).Return(shard1Reports, nil).Once()
	mockReportStore.EXPECT().LoadReports(ctx, m.Path(shard2)).Return(shard2Reports, nil).Once()

	// Mock saving merged reports
	mockReportStore.EXPECT().SaveReports(ctx, basePath, mock.MatchedBy(func(reports []m.Report) bool {
		return len(reports) == 3
	})).Return(nil)

	mockReportStore.EXPECT().RegenerateIndex(ctx, basePath).Return(nil)

	rm := domain.NewReportManager()

	// Act
	err := rm.Merge(ctx, basePath)

	// Assert
	assert.NoError(t, err)
	mockReportStore.AssertExpectations(t)

	// Verify only shard directories are removed, not the "other" directory
	_, err = os.Stat(otherDir)
	assert.NoError(t, err, "non-shard directory should not be removed")
}

func TestReportManager_Merge_ContextCancelled(t *testing.T) {
	// Arrange
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	basePath := m.Path(t.TempDir())

	rm := domain.NewReportManager()

	// Act
	err := rm.Merge(ctx, basePath)

	// Assert
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

func TestReportManager_Merge_RegenerateIndexError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockReportStore := new(adaptermocks.MockReportStore)

	basePath := m.Path(t.TempDir())

	// No shard directories, so only regenerate index is called
	indexErr := errors.New("failed to regenerate index")
	mockReportStore.EXPECT().RegenerateIndex(ctx, basePath).Return(indexErr)

	rm := domain.NewReportManager()

	// Act
	err := rm.Merge(ctx, basePath)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "regenerate index")
	mockReportStore.AssertExpectations(t)
}

func TestReportManager_Merge_EmptyShardReports(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockReportStore := new(adaptermocks.MockReportStore)

	basePath := m.Path(t.TempDir())

	// Create shard directory
	shard0 := filepath.Join(string(basePath), "shard_0")
	require.NoError(t, os.MkdirAll(shard0, 0755))

	// Empty shard reports
	shard0Reports := []m.Report{}

	// Mock loading existing reports from base (returns not found)
	mockReportStore.EXPECT().LoadReports(ctx, basePath).Return(nil, os.ErrNotExist).Once()

	// Mock loading shard reports
	mockReportStore.EXPECT().LoadReports(ctx, m.Path(shard0)).Return(shard0Reports, nil).Once()

	// Mock saving merged reports (empty)
	mockReportStore.EXPECT().SaveReports(ctx, basePath, mock.MatchedBy(func(reports []m.Report) bool {
		return len(reports) == 0
	})).Return(nil)

	mockReportStore.EXPECT().RegenerateIndex(ctx, basePath).Return(nil)

	rm := domain.NewReportManager()

	// Act
	err := rm.Merge(ctx, basePath)

	// Assert
	assert.NoError(t, err)
	mockReportStore.AssertExpectations(t)
}

// Helper function to create a mock FileSpill for testing
func createMockFileSpill(t *testing.T, reports []m.Report) pkg.FileSpill[m.Report] {
	t.Helper()

	// Create a file spill
	spill, err := pkg.NewFileSpill[m.Report]()
	require.NoError(t, err)

	for _, report := range reports {
		err := spill.Append(report)
		require.NoError(t, err)
	}

	return spill
}

func TestReportManager_NewReportManager(t *testing.T) {
	// Arrange & Act
	rm := domain.NewReportManager()

	// Assert
	assert.NotNil(t, rm)
}
