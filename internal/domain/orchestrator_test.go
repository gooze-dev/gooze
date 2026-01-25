package domain

import (
	"errors"
	"os"
	"testing"

	adaptermocks "github.com/mouse-blink/gooze/internal/adapter/mocks"
	m "github.com/mouse-blink/gooze/internal/model"
	"github.com/stretchr/testify/require"
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

	_, err := orch.TestMutation(mutation)
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

	result, err := orch.TestMutation(mutation)
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

	mutation := makeTestMutation()

	fsAdapter.EXPECT().FindProjectRoot(mutation.Source.Origin.FullPath).Return(m.Path(""), errors.New("root err"))

	_, err := orch.TestMutation(mutation)
	require.Error(t, err)
}

func TestOrchestrator_TestMutation_TestFailureMarksKilled(t *testing.T) {
	fsAdapter := adaptermocks.NewMockSourceFSAdapter(t)
	trAdapter := adaptermocks.NewMockTestRunnerAdapter(t)
	orch := NewOrchestrator(fsAdapter, trAdapter)

	mutation := makeTestMutation()
	projectRoot := m.Path("/project")
	tmpDir := m.Path("/tmp/mut")

	fsAdapter.EXPECT().FindProjectRoot(mutation.Source.Origin.FullPath).Return(projectRoot, nil)
	fsAdapter.EXPECT().CreateTempDir("gooze-mutation-*").Return(tmpDir, nil)
	fsAdapter.EXPECT().CopyDir(projectRoot, tmpDir).Return(nil)
	fsAdapter.EXPECT().RelPath(projectRoot, mutation.Source.Origin.FullPath).Return(m.Path("main.go"), nil)
	fsAdapter.EXPECT().JoinPath(string(tmpDir), "main.go").Return(m.Path("/tmp/mut/main.go"))
	fsAdapter.EXPECT().WriteFile(m.Path("/tmp/mut/main.go"), mutation.MutatedCode, os.FileMode(0o600)).Return(nil)
	fsAdapter.EXPECT().RelPath(projectRoot, mutation.Source.Test.FullPath).Return(m.Path("main_test.go"), nil)
	fsAdapter.EXPECT().JoinPath(string(tmpDir), "main_test.go").Return(m.Path("/tmp/mut/main_test.go"))
	fsAdapter.EXPECT().RemoveAll(tmpDir).Return(nil)
	trAdapter.EXPECT().RunGoTest("/tmp/mut", "/tmp/mut/main_test.go").Return("boom", errors.New("failed"))

	result, err := orch.TestMutation(mutation)
	require.NoError(t, err)

	entries, ok := result[mutation.Type]
	require.True(t, ok)
	require.Len(t, entries, 1)
	require.Equal(t, m.Killed, entries[0].Status)
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
