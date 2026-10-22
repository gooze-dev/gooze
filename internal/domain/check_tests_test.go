package domain_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	domain "gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
	m "gooze.dev/pkg/gooze/internal/model"
)

func TestOrchestrator_Overlay_CheckTestsRunsWithoutOverlay(t *testing.T) {
	fsAdapter := domainmocks.NewMockSourceFSAdapter(t)
	trAdapter := domainmocks.NewMockTestRunnerAdapter(t)
	ctx := context.Background()
	mutation := makeTestMutation()
	mutation.Source.Origin.ShortPath = "internal/max/main.go"

	fsAdapter.EXPECT().FindProjectRoot(ctx, m.Path("/project/main.go")).Return(m.Path("/project"), nil).Twice()
	trAdapter.EXPECT().RunGoTest(ctx, "/project", "/project/main.go", "").Return("ok", nil).Once()
	trAdapter.EXPECT().RunGoTest(ctx, "/project", "/project/main.go", "").Return("--- FAIL: TestMax\nFAIL\n", errors.New("exit status 1")).Once()

	ws := domain.NewOrchestrator(fsAdapter, trAdapter).NewWorkspace(domain.WorkspaceOverlay)
	defer ws.Close(ctx)

	require.NoError(t, ws.(domain.TestChecker).CheckTests(ctx, mutation))

	err := ws.(domain.TestChecker).CheckTests(ctx, mutation)

	var failing *domain.FailingTestsError
	require.ErrorAs(t, err, &failing)
	require.Equal(t, "./internal/max", failing.Package)
	require.Equal(t, "--- FAIL: TestMax\nFAIL\n", failing.Output)
	require.False(t, failing.TimedOut)
	require.EqualError(t, err, "tests of ./internal/max fail without any mutation")
}

func TestOrchestrator_DeepCopy_CheckTestsRunsInTheCopy(t *testing.T) {
	fsAdapter := domainmocks.NewMockSourceFSAdapter(t)
	trAdapter := domainmocks.NewMockTestRunnerAdapter(t)
	ctx, cancel := context.WithCancel(context.Background())
	mutation := makeTestMutation()
	mutation.Source.Origin.ShortPath = "main.go"
	projectRoot := m.Path("/project")
	tmpDir := m.Path("/tmp/mut")

	fsAdapter.EXPECT().FindProjectRoot(ctx, m.Path("/project/main.go")).Return(projectRoot, nil)
	fsAdapter.EXPECT().CreateTempDir(ctx, "gooze-mutation-*").Return(tmpDir, nil)
	fsAdapter.EXPECT().CopyDir(ctx, projectRoot, tmpDir).Return(nil)
	fsAdapter.EXPECT().RelPath(ctx, projectRoot, m.Path(mutation.Source.Test.FullPath)).Return(m.Path("main_test.go"), nil)
	fsAdapter.EXPECT().JoinPath(ctx, string(tmpDir), "main_test.go").Return(m.Path("/tmp/mut/main_test.go"))
	// The tests hang until the timeout cancels them.
	trAdapter.EXPECT().RunGoTest(ctx, "/tmp/mut", "/tmp/mut/main_test.go", "").
		RunAndReturn(func(context.Context, string, string, string) (string, error) {
			cancel()
			return "", context.Canceled
		})
	fsAdapter.EXPECT().RemoveAll(mock.Anything, tmpDir).Return(nil)

	ws := domain.NewOrchestrator(fsAdapter, trAdapter).NewWorkspace(domain.WorkspaceDeepCopy)
	defer ws.Close(ctx)

	err := ws.(domain.TestChecker).CheckTests(ctx, mutation)

	var failing *domain.FailingTestsError
	require.ErrorAs(t, err, &failing)
	require.Equal(t, ".", failing.Package)
	require.True(t, failing.TimedOut)
	require.EqualError(t, err, "tests of . don't finish within the mutation timeout without any mutation")
}
