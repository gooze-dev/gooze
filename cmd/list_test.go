package cmd

import (
	"bytes"
	"testing"

	"github.com/mouse-blink/gooze/internal/domain"
	domainmocks "github.com/mouse-blink/gooze/internal/domain/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
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

func TestNewListCmd(t *testing.T) {
	cmd := newListCmd()

	assert.Equal(t, "list [paths...]", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.Equal(t, listLongDescription, cmd.Long)
}
