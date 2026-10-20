package domain_test

import (
	"context"
	"errors"
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

	_, err := orch.TestMutation(context.Background(), mutation)
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

	result, err := orch.TestMutation(context.Background(), mutation)
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

	_, err := orch.TestMutation(ctx, mutation)
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
	trAdapter.EXPECT().RunGoTest(ctx, "/tmp/mut", "/tmp/mut/main_test.go").Return("boom", errors.New("failed"))
	fsAdapter.EXPECT().WriteFile(mock.Anything, m.Path("/tmp/mut/main.go"), original, os.FileMode(0o600)).Return(nil)
	fsAdapter.EXPECT().RemoveAll(mock.Anything, tmpDir).Return(nil)

	result, err := orch.TestMutation(ctx, mutation)
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

	result, err := orch.TestMutation(ctx, mutation)
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
	trAdapter.EXPECT().RunGoTest(ctx, "/tmp/mut", "/tmp/mut/main_test.go").
		Return("# example\n./main.go:2:1: syntax error\nFAIL\texample [build failed]\n", errors.New("exit status 1"))
	fsAdapter.EXPECT().WriteFile(mock.Anything, m.Path("/tmp/mut/main.go"), original, os.FileMode(0o600)).Return(nil)
	fsAdapter.EXPECT().RemoveAll(mock.Anything, tmpDir).Return(nil)

	result, err := orch.TestMutation(ctx, mutation)
	require.NoError(t, err)

	entries := result[mutation.Type]
	require.Len(t, entries, 1)
	require.Equal(t, rpt.Error, entries[0].Status)
}
