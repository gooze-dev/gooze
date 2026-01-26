package cmd

import (
	"bytes"
	"testing"

	"github.com/mouse-blink/gooze/internal/domain"
	domainmocks "github.com/mouse-blink/gooze/internal/domain/mocks"
	m "github.com/mouse-blink/gooze/internal/model"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestViewCmd_UsesRootOutputFlagByDefault(t *testing.T) {
	mockWorkflow := domainmocks.NewMockWorkflow(t)

	cmd := newRootCmd()
	cmd.AddCommand(newViewCmd())
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	originalWorkflow := workflow
	workflow = mockWorkflow
	defer func() { workflow = originalWorkflow }()

	mockWorkflow.On("View", mock.MatchedBy(func(args domain.ViewArgs) bool {
		return args.Reports == m.Path(".gooze-reports")
	})).Return(nil)

	cmd.SetArgs([]string{"view"})
	err := cmd.Execute()
	require.NoError(t, err)
}

func TestViewCmd_RootOutputFlagIsPassedThrough(t *testing.T) {
	mockWorkflow := domainmocks.NewMockWorkflow(t)

	cmd := newRootCmd()
	cmd.AddCommand(newViewCmd())
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	originalWorkflow := workflow
	workflow = mockWorkflow
	defer func() { workflow = originalWorkflow }()

	mockWorkflow.On("View", mock.MatchedBy(func(args domain.ViewArgs) bool {
		return args.Reports == m.Path("./reports-dir")
	})).Return(nil)

	cmd.SetArgs([]string{"--output", "./reports-dir", "view"})
	err := cmd.Execute()
	require.NoError(t, err)
}

func TestViewCmd_PositionalArgsAreRejected(t *testing.T) {
	mockWorkflow := domainmocks.NewMockWorkflow(t)

	cmd := newRootCmd()
	cmd.AddCommand(newViewCmd())
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	originalWorkflow := workflow
	workflow = mockWorkflow
	defer func() { workflow = originalWorkflow }()

	cmd.SetArgs([]string{"view", "./custom-reports"})
	err := cmd.Execute()
	require.Error(t, err)
}
