package cmd

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
)

func TestListCmd_UsesCache(t *testing.T) {
	mockWorkflow := domainmocks.NewMockWorkflow(t)

	cmd := newRootCmd()
	cmd.AddCommand(newListCmd())
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	originalWorkflow := workflow
	workflow = mockWorkflow
	defer func() { workflow = originalWorkflow }()

	mockWorkflow.On("Estimate", mock.MatchedBy(func(args domain.EstimateArgs) bool {
		return args.UseCache == true
	})).Return(nil)

	cmd.SetArgs([]string{"list", "./..."})
	err := cmd.Execute()
	require.NoError(t, err)

	mockWorkflow.AssertExpectations(t)
}

func TestListCmd_NoCacheFlag_DisablesCache(t *testing.T) {
	mockWorkflow := domainmocks.NewMockWorkflow(t)

	cmd := newRootCmd()
	cmd.AddCommand(newListCmd())
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	originalWorkflow := workflow
	workflow = mockWorkflow
	defer func() { workflow = originalWorkflow }()

	mockWorkflow.On("Estimate", mock.MatchedBy(func(args domain.EstimateArgs) bool {
		return args.UseCache == false
	})).Return(nil)

	cmd.SetArgs([]string{"--no-cache", "list", "./..."})
	err := cmd.Execute()
	require.NoError(t, err)

	mockWorkflow.AssertExpectations(t)
}

func TestListCmd_WithExcludePatterns(t *testing.T) {
	mockWorkflow := domainmocks.NewMockWorkflow(t)

	cmd := newRootCmd()
	cmd.AddCommand(newListCmd())
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	originalWorkflow := workflow
	workflow = mockWorkflow
	defer func() { workflow = originalWorkflow }()

	mockWorkflow.On("Estimate", mock.MatchedBy(func(args domain.EstimateArgs) bool {
		return len(args.Exclude) == 1 && args.Exclude[0] == "^vendor/"
	})).Return(nil)

	cmd.SetArgs([]string{"list", "-x", "^vendor/", "./..."})
	err := cmd.Execute()
	require.NoError(t, err)

	mockWorkflow.AssertExpectations(t)
}

func TestNewListCmd(t *testing.T) {
	cmd := newListCmd()

	assert.Equal(t, "list [paths...]", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.Equal(t, listLongDescription, cmd.Long)

	excludeFlag := cmd.Flags().Lookup("exclude")
	assert.NotNil(t, excludeFlag)
}
