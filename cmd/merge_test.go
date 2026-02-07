package cmd

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
	m "gooze.dev/pkg/gooze/internal/model"
)

func TestMergeCmd_UsesRootOutputFlagByDefault(t *testing.T) {
	mockWorkflow := domainmocks.NewMockWorkflow(t)

	cmd := newRootCmd()
	cmd.AddCommand(newMergeCmd())
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	originalWorkflow := workflow
	workflow = mockWorkflow
	defer func() { workflow = originalWorkflow }()

	mockWorkflow.On("Merge", mock.Anything, mock.MatchedBy(func(args domain.MergeArgs) bool {
		return args.Reports == m.Path(".gooze-reports")
	})).Return(nil)

	cmd.SetArgs([]string{"merge"})
	err := cmd.Execute()
	require.NoError(t, err)
}

func TestMergeCmd_RootOutputFlagIsPassedThrough(t *testing.T) {
	mockWorkflow := domainmocks.NewMockWorkflow(t)

	cmd := newRootCmd()
	cmd.AddCommand(newMergeCmd())
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	originalWorkflow := workflow
	workflow = mockWorkflow
	defer func() { workflow = originalWorkflow }()

	mockWorkflow.On("Merge", mock.Anything, mock.MatchedBy(func(args domain.MergeArgs) bool {
		return args.Reports == m.Path("./reports-dir")
	})).Return(nil)

	cmd.SetArgs([]string{"--output", "./reports-dir", "merge"})
	err := cmd.Execute()
	require.NoError(t, err)
}
