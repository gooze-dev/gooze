package cmd

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
	m "gooze.dev/pkg/gooze/internal/model"
)

func TestRunCmd_TestMode(t *testing.T) {
	mockWorkflow := domainmocks.NewMockWorkflow(t)

	cmd := newRootCmd()
	cmd.AddCommand(newRunCmd())
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	originalWorkflow := workflow
	workflow = mockWorkflow
	defer func() { workflow = originalWorkflow }()

	mockWorkflow.On("TestStream", mock.Anything, mock.MatchedBy(func(args domain.TestArgs) bool {
		return args.Threads == 2 &&
			args.ShardIndex == 0 &&
			args.TotalShardCount == 1 &&
			args.Reports == m.Path(".gooze-reports") &&
			args.MutationTimeout == 120*time.Second
	})).Return(nil)

	cmd.SetArgs([]string{"run", "--parallel", "2", "./..."})
	err := cmd.Execute()
	require.NoError(t, err)

	mockWorkflow.AssertExpectations(t)
}

func TestRunCmd_WithSharding(t *testing.T) {
	mockWorkflow := domainmocks.NewMockWorkflow(t)

	cmd := newRootCmd()
	cmd.AddCommand(newRunCmd())
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	originalWorkflow := workflow
	workflow = mockWorkflow
	defer func() { workflow = originalWorkflow }()

	mockWorkflow.On("TestStream", mock.Anything, mock.MatchedBy(func(args domain.TestArgs) bool {
		return args.ShardIndex == 1 && args.TotalShardCount == 3
	})).Return(nil)

	cmd.SetArgs([]string{"run", "--shard", "1/3", "./..."})
	err := cmd.Execute()
	require.NoError(t, err)

	mockWorkflow.AssertExpectations(t)
}

func TestRunCmd_MultiplePaths(t *testing.T) {
	mockWorkflow := domainmocks.NewMockWorkflow(t)

	cmd := newRootCmd()
	cmd.AddCommand(newRunCmd())
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	originalWorkflow := workflow
	workflow = mockWorkflow
	defer func() { workflow = originalWorkflow }()

	mockWorkflow.On("TestStream", mock.Anything, mock.MatchedBy(func(args domain.TestArgs) bool {
		return len(args.Paths) == 3 &&
			args.Paths[0] == m.Path("./cmd") &&
			args.Paths[1] == m.Path("./pkg") &&
			args.Paths[2] == m.Path("./internal")
	})).Return(nil)

	cmd.SetArgs([]string{"run", "./cmd", "./pkg", "./internal"})
	err := cmd.Execute()
	require.NoError(t, err)

	mockWorkflow.AssertExpectations(t)
}

func TestRunCmd_WithExcludePatterns(t *testing.T) {
	mockWorkflow := domainmocks.NewMockWorkflow(t)

	cmd := newRootCmd()
	cmd.AddCommand(newRunCmd())
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	originalWorkflow := workflow
	workflow = mockWorkflow
	defer func() { workflow = originalWorkflow }()

	mockWorkflow.On("TestStream", mock.Anything, mock.MatchedBy(func(args domain.TestArgs) bool {
		return len(args.Exclude) == 2 &&
			args.Exclude[0] == "^generated_" &&
			args.Exclude[1] == "_gen\\.go$"
	})).Return(nil)

	cmd.SetArgs([]string{"run", "-x", "^generated_", "-x", "_gen\\.go$", "./..."})
	err := cmd.Execute()
	require.NoError(t, err)

	mockWorkflow.AssertExpectations(t)
}

func TestRunCmd_NoCacheFlag_DisablesCache(t *testing.T) {
	mockWorkflow := domainmocks.NewMockWorkflow(t)

	cmd := newRootCmd()
	cmd.AddCommand(newRunCmd())
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	originalWorkflow := workflow
	workflow = mockWorkflow
	defer func() { workflow = originalWorkflow }()

	mockWorkflow.On("TestStream", mock.Anything, mock.MatchedBy(func(args domain.TestArgs) bool {
		return args.UseCache == false
	})).Return(nil)

	cmd.SetArgs([]string{"--no-cache", "run", "./..."})
	err := cmd.Execute()
	require.NoError(t, err)

	mockWorkflow.AssertExpectations(t)
}

func TestNewRunCmd(t *testing.T) {
	cmd := newRunCmd()

	assert.Equal(t, "run [paths...]", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.Equal(t, runLongDescription, cmd.Long)

	parallelFlag := cmd.Flags().Lookup("parallel")
	assert.NotNil(t, parallelFlag)
	shardFlag := cmd.Flags().Lookup("shard")
	assert.NotNil(t, shardFlag)
}
