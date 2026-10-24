package domain_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	domain "gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
	m "gooze.dev/pkg/gooze/internal/model"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

func TestOrchestrator_Overlay_NoTestFilesIsNotCovered(t *testing.T) {
	fsAdapter := domainmocks.NewMockSourceFSAdapter(t)
	trAdapter := domainmocks.NewMockTestRunnerAdapter(t)
	orch := domain.NewOrchestrator(fsAdapter, trAdapter)
	ctx := context.Background()
	mutation := makeTestMutation()

	fsAdapter.EXPECT().FindProjectRoot(ctx, m.Path("/project/main.go")).Return(m.Path("/project"), nil)
	fsAdapter.EXPECT().CreateTempDir(ctx, "gooze-overlay-*").Return(m.Path("/tmp/ovl"), nil)
	fsAdapter.EXPECT().JoinPath(ctx, "/tmp/ovl", mock.Anything).RunAndReturn(func(_ context.Context, elem ...string) m.Path {
		return m.Path(elem[0] + "/" + elem[1])
	})
	fsAdapter.EXPECT().WriteFile(ctx, mock.Anything, mock.Anything, os.FileMode(0o600)).Return(nil)
	// Test files behind build tags this build doesn't match: go test runs nothing.
	trAdapter.EXPECT().RunGoTest(ctx, "/project", "/project/main.go", "/tmp/ovl/overlay.json").
		Return("?   \texample\t[no test files]\n", nil)
	fsAdapter.EXPECT().RemoveAll(mock.Anything, m.Path("/tmp/ovl")).Return(nil)

	result, err := orch.TestMutation(ctx, domain.WorkspaceOverlay, mutation)
	require.NoError(t, err)
	require.Equal(t, rpt.NotCovered, result[mutation.Type][0].Status)
}
