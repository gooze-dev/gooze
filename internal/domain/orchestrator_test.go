package domain_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	domain "gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
	m "gooze.dev/pkg/gooze/internal/model"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

func TestOrchestrator_TestMutation_NoOrigin(t *testing.T) {
	orch := domain.NewOrchestrator(nil, nil)
	mutation := m.Mutation{
		ID:   "hash-1",
		Type: mgen.Arithmetic,
		Source: rpt.Source{
			Origin: nil,
			Test:   &rpt.File{FullPath: "/project/main_test.go"},
		},
	}

	_, err := orch.TestMutation(context.Background(), domain.WorkspaceOverlay, mutation)
	require.Error(t, err)
}

func TestOrchestrator_TestMutation_NoTestFile(t *testing.T) {
	orch := domain.NewOrchestrator(nil, nil)

	mutation := m.Mutation{
		ID:   "test-hash-id",
		Type: mgen.Boolean,
		Source: rpt.Source{
			Origin: &rpt.File{FullPath: "/project/main.go"},
			Test:   nil,
		},
	}

	result, err := orch.TestMutation(context.Background(), domain.WorkspaceOverlay, mutation)
	require.NoError(t, err)

	entries, ok := result[mutation.Type]
	require.True(t, ok)
	require.Len(t, entries, 1)
	require.Equal(t, "test-hash-id", entries[0].MutationID)
	require.Equal(t, rpt.Survived, entries[0].Status)
}

func TestOrchestrator_TestMutation_FindProjectRootError(t *testing.T) {
	fsAdapter := domainmocks.NewMockSourceFSAdapter(t)
	trAdapter := domainmocks.NewMockTestRunnerAdapter(t)
	orch := domain.NewOrchestrator(fsAdapter, trAdapter)
	ctx := context.Background()
	mutation := makeTestMutation()

	fsAdapter.EXPECT().FindProjectRoot(ctx, m.Path(mutation.Source.Origin.FullPath)).Return(m.Path(""), errors.New("root err"))

	_, err := orch.TestMutation(ctx, domain.WorkspaceDeepCopy, mutation)
	require.Error(t, err)
}

func TestOrchestrator_TestMutation_TestFailureMarksKilled(t *testing.T) {
	fsAdapter := domainmocks.NewMockSourceFSAdapter(t)
	trAdapter := domainmocks.NewMockTestRunnerAdapter(t)
	orch := domain.NewOrchestrator(fsAdapter, trAdapter)
	ctx := context.Background()
	mutation := makeTestMutation()
	projectRoot := m.Path("/project")
	tmpDir := m.Path("/tmp/mut")

	original := []byte("package main\nfunc main() { _ = 1 + 2 }\n")

	fsAdapter.EXPECT().FindProjectRoot(ctx, m.Path(mutation.Source.Origin.FullPath)).Return(projectRoot, nil)
	fsAdapter.EXPECT().CreateTempDir(ctx, "gooze-mutation-*").Return(tmpDir, nil)
	fsAdapter.EXPECT().CopyDir(ctx, projectRoot, tmpDir).Return(nil)
	fsAdapter.EXPECT().RelPath(ctx, projectRoot, m.Path(mutation.Source.Origin.FullPath)).Return(m.Path("main.go"), nil)
	fsAdapter.EXPECT().JoinPath(ctx, string(tmpDir), "main.go").Return(m.Path("/tmp/mut/main.go"))
	fsAdapter.EXPECT().RelPath(ctx, projectRoot, m.Path(mutation.Source.Test.FullPath)).Return(m.Path("main_test.go"), nil)
	fsAdapter.EXPECT().JoinPath(ctx, string(tmpDir), "main_test.go").Return(m.Path("/tmp/mut/main_test.go"))
	// Save the original, write the mutation, then restore the original so the
	// workspace can be reused (restore runs under a cancellation-free context).
	fsAdapter.EXPECT().ReadFile(ctx, m.Path("/tmp/mut/main.go")).Return(original, nil)
	fsAdapter.EXPECT().WriteFile(ctx, m.Path("/tmp/mut/main.go"), mutation.MutatedCode, os.FileMode(0o600)).Return(nil)
	trAdapter.EXPECT().RunGoTest(ctx, "/tmp/mut", "/tmp/mut/main_test.go", "").Return("boom", errors.New("failed"))
	fsAdapter.EXPECT().WriteFile(mock.Anything, m.Path("/tmp/mut/main.go"), original, os.FileMode(0o600)).Return(nil)
	fsAdapter.EXPECT().RemoveAll(mock.Anything, tmpDir).Return(nil)

	result, err := orch.TestMutation(ctx, domain.WorkspaceDeepCopy, mutation)
	require.NoError(t, err)

	entries, ok := result[mutation.Type]
	require.True(t, ok)
	require.Len(t, entries, 1)
	require.Equal(t, rpt.Killed, entries[0].Status)
}

func TestOrchestrator_TestMutation_ContextCancelledReturnsTimeout(t *testing.T) {
	orch := domain.NewOrchestrator(nil, nil)
	mutation := makeTestMutation()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel the context immediately

	result, err := orch.TestMutation(ctx, domain.WorkspaceDeepCopy, mutation)
	require.NoError(t, err)

	entries, ok := result[mutation.Type]
	require.True(t, ok)
	require.Len(t, entries, 1)
	require.Equal(t, rpt.Timeout, entries[0].Status)
}

func makeTestMutation() m.Mutation {
	return m.Mutation{
		ID:          "test-mutation-hash",
		Type:        mgen.Arithmetic,
		MutatedCode: []byte("package main\nfunc main() { _ = 1 + 1 }\n"),
		Source: rpt.Source{
			Origin: &rpt.File{FullPath: "/project/main.go"},
			Test:   &rpt.File{FullPath: "/project/main_test.go"},
		},
	}
}

func TestOrchestrator_TestMutation_BuildFailureMarksError(t *testing.T) {
	fsAdapter := domainmocks.NewMockSourceFSAdapter(t)
	trAdapter := domainmocks.NewMockTestRunnerAdapter(t)
	orch := domain.NewOrchestrator(fsAdapter, trAdapter)
	ctx := context.Background()
	mutation := makeTestMutation()
	projectRoot := m.Path("/project")
	tmpDir := m.Path("/tmp/mut")
	original := []byte("package main\nfunc main() { _ = 1 + 2 }\n")

	fsAdapter.EXPECT().FindProjectRoot(ctx, m.Path(mutation.Source.Origin.FullPath)).Return(projectRoot, nil)
	fsAdapter.EXPECT().CreateTempDir(ctx, "gooze-mutation-*").Return(tmpDir, nil)
	fsAdapter.EXPECT().CopyDir(ctx, projectRoot, tmpDir).Return(nil)
	fsAdapter.EXPECT().RelPath(ctx, projectRoot, m.Path(mutation.Source.Origin.FullPath)).Return(m.Path("main.go"), nil)
	fsAdapter.EXPECT().JoinPath(ctx, string(tmpDir), "main.go").Return(m.Path("/tmp/mut/main.go"))
	fsAdapter.EXPECT().RelPath(ctx, projectRoot, m.Path(mutation.Source.Test.FullPath)).Return(m.Path("main_test.go"), nil)
	fsAdapter.EXPECT().JoinPath(ctx, string(tmpDir), "main_test.go").Return(m.Path("/tmp/mut/main_test.go"))
	fsAdapter.EXPECT().ReadFile(ctx, m.Path("/tmp/mut/main.go")).Return(original, nil)
	fsAdapter.EXPECT().WriteFile(ctx, m.Path("/tmp/mut/main.go"), mutation.MutatedCode, os.FileMode(0o600)).Return(nil)
	trAdapter.EXPECT().RunGoTest(ctx, "/tmp/mut", "/tmp/mut/main_test.go", "").
		Return("# example\n./main.go:2:1: syntax error\nFAIL\texample [build failed]\n", errors.New("exit status 1"))
	fsAdapter.EXPECT().WriteFile(mock.Anything, m.Path("/tmp/mut/main.go"), original, os.FileMode(0o600)).Return(nil)
	fsAdapter.EXPECT().RemoveAll(mock.Anything, tmpDir).Return(nil)

	result, err := orch.TestMutation(ctx, domain.WorkspaceDeepCopy, mutation)
	require.NoError(t, err)

	entries := result[mutation.Type]
	require.Len(t, entries, 1)
	require.Equal(t, rpt.Error, entries[0].Status)
}

func TestOrchestrator_Overlay_TestFailureMarksKilled(t *testing.T) {
	fsAdapter := domainmocks.NewMockSourceFSAdapter(t)
	trAdapter := domainmocks.NewMockTestRunnerAdapter(t)
	orch := domain.NewOrchestrator(fsAdapter, trAdapter)
	ctx := context.Background()
	mutation := makeTestMutation()
	tmpDir := m.Path("/tmp/ovl")

	fsAdapter.EXPECT().FindProjectRoot(ctx, m.Path("/project/main.go")).Return(m.Path("/project"), nil)
	fsAdapter.EXPECT().CreateTempDir(ctx, "gooze-overlay-*").Return(tmpDir, nil)
	fsAdapter.EXPECT().JoinPath(ctx, string(tmpDir), "mutant.go").Return(m.Path("/tmp/ovl/mutant.go"))
	fsAdapter.EXPECT().JoinPath(ctx, string(tmpDir), "overlay.json").Return(m.Path("/tmp/ovl/overlay.json"))
	// The mutant goes to the temp dir; the project's own file is never written.
	fsAdapter.EXPECT().WriteFile(ctx, m.Path("/tmp/ovl/mutant.go"), mutation.MutatedCode, os.FileMode(0o600)).Return(nil)
	fsAdapter.EXPECT().WriteFile(ctx, m.Path("/tmp/ovl/overlay.json"),
		[]byte(`{"Replace":{"/project/main.go":"/tmp/ovl/mutant.go"}}`), os.FileMode(0o600)).Return(nil)
	trAdapter.EXPECT().RunGoTest(ctx, "/project", "/project/main.go", "/tmp/ovl/overlay.json").Return("boom", errors.New("failed"))
	fsAdapter.EXPECT().RemoveAll(mock.Anything, tmpDir).Return(nil)

	result, err := orch.TestMutation(ctx, domain.WorkspaceOverlay, mutation)
	require.NoError(t, err)
	require.Equal(t, rpt.Killed, result[mutation.Type][0].Status)
}

func TestOrchestrator_Overlay_BuildFailureMarksError(t *testing.T) {
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
	trAdapter.EXPECT().RunGoTest(ctx, "/project", "/project/main.go", "/tmp/ovl/overlay.json").
		Return("# example\n./main.go:2:1: syntax error\nFAIL\texample [build failed]\n", errors.New("exit status 1"))
	fsAdapter.EXPECT().RemoveAll(mock.Anything, m.Path("/tmp/ovl")).Return(nil)

	result, err := orch.TestMutation(ctx, domain.WorkspaceOverlay, mutation)
	require.NoError(t, err)
	require.Equal(t, rpt.Error, result[mutation.Type][0].Status)
}

func TestOrchestrator_Overlay_ReusesTempDirAcrossMutations(t *testing.T) {
	fsAdapter := domainmocks.NewMockSourceFSAdapter(t)
	trAdapter := domainmocks.NewMockTestRunnerAdapter(t)
	orch := domain.NewOrchestrator(fsAdapter, trAdapter)
	ctx := context.Background()

	fsAdapter.EXPECT().FindProjectRoot(ctx, m.Path("/project/main.go")).Return(m.Path("/project"), nil).Times(2)
	fsAdapter.EXPECT().CreateTempDir(ctx, "gooze-overlay-*").Return(m.Path("/tmp/ovl"), nil).Once()
	fsAdapter.EXPECT().JoinPath(ctx, "/tmp/ovl", mock.Anything).RunAndReturn(func(_ context.Context, elem ...string) m.Path {
		return m.Path(elem[0] + "/" + elem[1])
	})
	fsAdapter.EXPECT().WriteFile(ctx, mock.Anything, mock.Anything, os.FileMode(0o600)).Return(nil)
	trAdapter.EXPECT().RunGoTest(ctx, "/project", "/project/main.go", "/tmp/ovl/overlay.json").Return("ok", nil).Times(2)
	fsAdapter.EXPECT().RemoveAll(mock.Anything, m.Path("/tmp/ovl")).Return(nil).Once()

	ws := orch.NewWorkspace(domain.WorkspaceOverlay)

	for range 2 {
		result, err := ws.Run(ctx, makeTestMutation())
		require.NoError(t, err)
		require.Equal(t, rpt.Survived, result[mgen.Arithmetic][0].Status)
	}

	ws.Close(ctx)
}

func TestOrchestrator_EmptyModeUsesOverlay(t *testing.T) {
	fsAdapter := domainmocks.NewMockSourceFSAdapter(t)
	trAdapter := domainmocks.NewMockTestRunnerAdapter(t)
	orch := domain.NewOrchestrator(fsAdapter, trAdapter)
	ctx := context.Background()

	fsAdapter.EXPECT().FindProjectRoot(ctx, m.Path("/project/main.go")).Return(m.Path("/project"), nil)
	fsAdapter.EXPECT().CreateTempDir(ctx, "gooze-overlay-*").Return(m.Path("/tmp/ovl"), nil)
	fsAdapter.EXPECT().JoinPath(ctx, "/tmp/ovl", mock.Anything).RunAndReturn(func(_ context.Context, elem ...string) m.Path {
		return m.Path(elem[0] + "/" + elem[1])
	})
	fsAdapter.EXPECT().WriteFile(ctx, mock.Anything, mock.Anything, os.FileMode(0o600)).Return(nil)
	trAdapter.EXPECT().RunGoTest(ctx, "/project", "/project/main.go", "/tmp/ovl/overlay.json").Return("ok", nil)
	fsAdapter.EXPECT().RemoveAll(mock.Anything, m.Path("/tmp/ovl")).Return(nil)

	_, err := orch.TestMutation(ctx, "", makeTestMutation())
	require.NoError(t, err)
}

func TestOrchestrator_Overlay_CloseLogsRemoveFailure(t *testing.T) {
	var logs bytes.Buffer

	defaultLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(defaultLogger) })

	fsAdapter := domainmocks.NewMockSourceFSAdapter(t)
	trAdapter := domainmocks.NewMockTestRunnerAdapter(t)
	orch := domain.NewOrchestrator(fsAdapter, trAdapter)
	ctx := context.Background()

	fsAdapter.EXPECT().FindProjectRoot(ctx, m.Path("/project/main.go")).Return(m.Path("/project"), nil)
	fsAdapter.EXPECT().CreateTempDir(ctx, "gooze-overlay-*").Return(m.Path("/tmp/ovl"), nil)
	fsAdapter.EXPECT().JoinPath(ctx, "/tmp/ovl", mock.Anything).RunAndReturn(func(_ context.Context, elem ...string) m.Path {
		return m.Path(elem[0] + "/" + elem[1])
	})
	fsAdapter.EXPECT().WriteFile(ctx, mock.Anything, mock.Anything, os.FileMode(0o600)).Return(nil)
	trAdapter.EXPECT().RunGoTest(ctx, "/project", "/project/main.go", "/tmp/ovl/overlay.json").Return("ok", nil)
	fsAdapter.EXPECT().RemoveAll(mock.Anything, m.Path("/tmp/ovl")).Return(errors.New("busy"))

	_, err := orch.TestMutation(ctx, domain.WorkspaceOverlay, makeTestMutation())
	require.NoError(t, err)
	require.Contains(t, logs.String(), "Failed to cleanup temp dir")
	require.Contains(t, logs.String(), "busy")
}

func TestOrchestrator_Overlay_ContextCancelledReturnsTimeout(t *testing.T) {
	// No adapters: a canceled run must return before touching any of them.
	orch := domain.NewOrchestrator(nil, nil)
	mutation := makeTestMutation()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := orch.TestMutation(ctx, domain.WorkspaceOverlay, mutation)
	require.NoError(t, err)
	require.Equal(t, rpt.Timeout, result[mutation.Type][0].Status)
}
