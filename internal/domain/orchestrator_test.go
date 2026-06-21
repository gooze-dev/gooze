package domain

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	adaptermocks "gooze.dev/pkg/gooze/internal/adapter/mocks"
	m "gooze.dev/pkg/gooze/internal/model"
)

func TestOrchestrator_TestMutation_NoOrigin(t *testing.T) {
	orch := NewOrchestrator(nil, nil)
	mutation := m.Mutation{
		ID:   "hash-1",
		Type: m.MutationArithmetic,
		Source: m.Source{
			Origin: nil,
			Test:   &m.File{FullPath: m.Path("/project/main_test.go")},
		},
	}

	_, err := orch.TestMutation(context.Background(), mutation)
	require.Error(t, err)
}

func TestOrchestrator_TestMutation_NoTestFile(t *testing.T) {
	orch := NewOrchestrator(nil, nil)

	mutation := m.Mutation{
		ID:   "test-hash-id",
		Type: m.MutationBoolean,
		Source: m.Source{
			Origin: &m.File{FullPath: m.Path("/project/main.go")},
			Test:   nil,
		},
	}

	result, err := orch.TestMutation(context.Background(), mutation)
	require.NoError(t, err)

	entries, ok := result[mutation.Type]
	require.True(t, ok)
	require.Len(t, entries, 1)
	require.Equal(t, "test-hash-id", entries[0].MutationID)
	require.Equal(t, m.Survived, entries[0].Status)
}

func TestOrchestrator_TestMutation_FindProjectRootError(t *testing.T) {
	fsAdapter := adaptermocks.NewMockSourceFSAdapter(t)
	trAdapter := adaptermocks.NewMockTestRunnerAdapter(t)
	orch := NewOrchestrator(fsAdapter, trAdapter)
	ctx := context.Background()
	mutation := makeTestMutation()

	fsAdapter.EXPECT().FindProjectRoot(ctx, mutation.Source.Origin.FullPath).Return(m.Path(""), errors.New("root err"))

	_, err := orch.TestMutation(ctx, mutation)
	require.Error(t, err)
}

func TestOrchestrator_TestMutation_TestFailureMarksKilled(t *testing.T) {
	fsAdapter := adaptermocks.NewMockSourceFSAdapter(t)
	trAdapter := adaptermocks.NewMockTestRunnerAdapter(t)
	orch := NewOrchestrator(fsAdapter, trAdapter)
	ctx := context.Background()
	mutation := makeTestMutation()
	projectRoot := m.Path("/project")
	tmpDir := m.Path("/tmp/mut")

	original := []byte("package main\nfunc main() { _ = 1 + 2 }\n")

	fsAdapter.EXPECT().FindProjectRoot(ctx, mutation.Source.Origin.FullPath).Return(projectRoot, nil)
	fsAdapter.EXPECT().CreateTempDir(ctx, "gooze-mutation-*").Return(tmpDir, nil)
	fsAdapter.EXPECT().CopyDir(ctx, projectRoot, tmpDir).Return(nil)
	fsAdapter.EXPECT().RelPath(ctx, projectRoot, mutation.Source.Origin.FullPath).Return(m.Path("main.go"), nil)
	fsAdapter.EXPECT().JoinPath(ctx, string(tmpDir), "main.go").Return(m.Path("/tmp/mut/main.go"))
	fsAdapter.EXPECT().RelPath(ctx, projectRoot, mutation.Source.Test.FullPath).Return(m.Path("main_test.go"), nil)
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
	require.Equal(t, m.Killed, entries[0].Status)
}

func TestOrchestrator_TestMutation_ContextCancelledReturnsTimeout(t *testing.T) {
	orch := NewOrchestrator(nil, nil)
	mutation := makeTestMutation()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel the context immediately

	result, err := orch.TestMutation(ctx, mutation)
	require.NoError(t, err)

	entries, ok := result[mutation.Type]
	require.True(t, ok)
	require.Len(t, entries, 1)
	require.Equal(t, m.Timeout, entries[0].Status)
}

func TestInShard(t *testing.T) {
	ids := []string{"hash-0", "hash-1", "hash-2", "hash-3", "hash-4", "hash-5"}

	// A non-positive total disables sharding: everything is included.
	for _, id := range ids {
		require.True(t, inShard(id, 0, 0))
		require.True(t, inShard(id, 0, -3))
	}

	// An out-of-range shard index matches nothing.
	for _, id := range ids {
		require.False(t, inShard(id, 2, 2))
		require.False(t, inShard(id, 3, 2))
	}

	// Each id belongs to exactly one shard of a valid partition.
	const total = 3
	for _, id := range ids {
		matches := 0
		for shard := range total {
			if inShard(id, shard, total) {
				matches++
			}
		}

		require.Equal(t, 1, matches, "id %q must belong to exactly one shard", id)
	}
}

func makeTestMutation() m.Mutation {
	return m.Mutation{
		ID:          "test-mutation-hash",
		Type:        m.MutationArithmetic,
		MutatedCode: []byte("package main\nfunc main() { _ = 1 + 1 }\n"),
		Source: m.Source{
			Origin: &m.File{FullPath: m.Path("/project/main.go")},
			Test:   &m.File{FullPath: m.Path("/project/main_test.go")},
		},
	}
}
