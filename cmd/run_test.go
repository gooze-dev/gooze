package cmd

import (
	"bytes"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
	m "gooze.dev/pkg/gooze/internal/model"
	"gooze.dev/pkg/gooze/pkg/config"
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

	mockWorkflow.On("Test", mock.Anything, mock.MatchedBy(func(args domain.TestArgs) bool {
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

func TestRunCmd_RejectsOutOfRangeParallel(t *testing.T) {
	mockWorkflow := domainmocks.NewMockWorkflow(t)

	cmd := newRootCmd()
	cmd.AddCommand(newRunCmd())
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	originalWorkflow := workflow
	workflow = mockWorkflow
	defer func() { workflow = originalWorkflow }()
	// Rebind run.parallel to a fresh, unset flag so later tests see the default.
	defer newRunCmd()

	cmd.SetArgs([]string{"run", "--parallel", "0", "./..."})
	err := cmd.Execute()
	require.EqualError(t, err, "invalid run.parallel: 0 is out of range: must be at least 1")
}

func TestRunCmd_CoverageProfileFlag(t *testing.T) {
	mockWorkflow := domainmocks.NewMockWorkflow(t)

	cmd := newRootCmd()
	cmd.AddCommand(newRunCmd())
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	originalWorkflow := workflow
	workflow = mockWorkflow
	defer func() { workflow = originalWorkflow }()
	defer viper.Set(config.KeyRunCoverageProfile, "")

	mockWorkflow.On("Test", mock.Anything, mock.MatchedBy(func(args domain.TestArgs) bool {
		return args.CoverageProfile == m.Path("coverage.out")
	})).Return(nil)

	cmd.SetArgs([]string{"run", "--coverage-profile", "coverage.out", "./..."})
	require.NoError(t, cmd.Execute())

	mockWorkflow.AssertExpectations(t)
}

func TestRunCmd_CoverageProfileDefaultsEmpty(t *testing.T) {
	mockWorkflow := domainmocks.NewMockWorkflow(t)

	cmd := newRootCmd()
	cmd.AddCommand(newRunCmd())
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	originalWorkflow := workflow
	workflow = mockWorkflow
	defer func() { workflow = originalWorkflow }()

	mockWorkflow.On("Test", mock.Anything, mock.MatchedBy(func(args domain.TestArgs) bool {
		return args.CoverageProfile == ""
	})).Return(nil)

	cmd.SetArgs([]string{"run", "./..."})
	require.NoError(t, cmd.Execute())

	mockWorkflow.AssertExpectations(t)
}

// diffRunCmd runs `gooze <args>` against a mock workflow and returns it.
func diffRunCmd(t *testing.T, mockWorkflow *domainmocks.MockWorkflow, args ...string) error {
	t.Helper()

	cmd := newRootCmd()
	cmd.AddCommand(newRunCmd())
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	originalWorkflow := workflow
	workflow = mockWorkflow
	t.Cleanup(func() { workflow = originalWorkflow })
	// Rebind run.diff to a fresh, unset flag so this run's --diff doesn't
	// leak into later tests (viper.Set would override every layer instead).
	t.Cleanup(func() { newRunCmd() })

	// --run and --mutagens are plain flags kept in package variables; reset them.
	t.Cleanup(func() { runPatternFlag, runMutagensFlag = "", nil })

	cmd.SetArgs(args)

	return cmd.Execute()
}

func TestRunCmd_DiffFlag(t *testing.T) {
	mockWorkflow := domainmocks.NewMockWorkflow(t)
	mockWorkflow.On("Test", mock.Anything, mock.MatchedBy(func(args domain.TestArgs) bool {
		return args.Diff == "origin/main"
	})).Return(nil)

	require.NoError(t, diffRunCmd(t, mockWorkflow, "run", "--diff", "origin/main", "./..."))
}

func TestRunCmd_DiffFlagWithEstimate(t *testing.T) {
	mockWorkflow := domainmocks.NewMockWorkflow(t)
	mockWorkflow.On("Estimate", mock.Anything, mock.MatchedBy(func(args domain.EstimateArgs) bool {
		return args.Diff == "HEAD~1"
	})).Return(nil)

	require.NoError(t, diffRunCmd(t, mockWorkflow, "run", "--estimate", "--diff", "HEAD~1"))
}

func TestRunCmd_DiffDefaultsOff(t *testing.T) {
	mockWorkflow := domainmocks.NewMockWorkflow(t)
	mockWorkflow.On("Test", mock.Anything, mock.MatchedBy(func(args domain.TestArgs) bool {
		return args.Diff == ""
	})).Return(nil)

	require.NoError(t, diffRunCmd(t, mockWorkflow, "run", "./..."))
}

// setConfigDiff sets run.diff in viper's config-file layer, as gooze.yaml does.
func setConfigDiff(t *testing.T, ref string) {
	t.Helper()
	require.NoError(t, viper.MergeConfigMap(map[string]any{"run": map[string]any{"diff": ref}}))
	t.Cleanup(func() { _ = viper.MergeConfigMap(map[string]any{"run": map[string]any{"diff": ""}}) })
}

func TestRunCmd_DiffFromConfig(t *testing.T) {
	setConfigDiff(t, "origin/develop")

	mockWorkflow := domainmocks.NewMockWorkflow(t)
	mockWorkflow.On("Test", mock.Anything, mock.MatchedBy(func(args domain.TestArgs) bool {
		return args.Diff == "origin/develop"
	})).Return(nil)

	require.NoError(t, diffRunCmd(t, mockWorkflow, "run", "./..."))
}

func TestRunCmd_DiffFlagOverridesConfig(t *testing.T) {
	setConfigDiff(t, "origin/develop")

	mockWorkflow := domainmocks.NewMockWorkflow(t)
	mockWorkflow.On("Test", mock.Anything, mock.MatchedBy(func(args domain.TestArgs) bool {
		return args.Diff == ""
	})).Return(nil)

	// An explicit empty value turns a configured diff off for one run.
	require.NoError(t, diffRunCmd(t, mockWorkflow, "run", "--diff=", "./..."))
}

func TestRunCmd_DiffFromEnv(t *testing.T) {
	t.Setenv("GOOZE_RUN_DIFF", "main")

	mockWorkflow := domainmocks.NewMockWorkflow(t)
	mockWorkflow.On("Test", mock.Anything, mock.MatchedBy(func(args domain.TestArgs) bool {
		return args.Diff == "main"
	})).Return(nil)

	require.NoError(t, diffRunCmd(t, mockWorkflow, "run", "./..."))
}

func TestRunCmd_RunFlag(t *testing.T) {
	mockWorkflow := domainmocks.NewMockWorkflow(t)
	mockWorkflow.On("Test", mock.Anything, mock.MatchedBy(func(args domain.TestArgs) bool {
		return args.Run == "^InRange$"
	})).Return(nil)

	require.NoError(t, diffRunCmd(t, mockWorkflow, "run", "--run", "^InRange$", "./calc"))
}

func TestRunCmd_RunFlagWithEstimate(t *testing.T) {
	mockWorkflow := domainmocks.NewMockWorkflow(t)
	mockWorkflow.On("Estimate", mock.Anything, mock.MatchedBy(func(args domain.EstimateArgs) bool {
		return args.Run == "Parser\\.Next"
	})).Return(nil)

	require.NoError(t, diffRunCmd(t, mockWorkflow, "run", "--estimate", "--run", "Parser\\.Next"))
}

func TestRunCmd_MutagensFlag(t *testing.T) {
	mockWorkflow := domainmocks.NewMockWorkflow(t)
	mockWorkflow.On("Test", mock.Anything, mock.MatchedBy(func(args domain.TestArgs) bool {
		return assert.ObjectsAreEqual([]string{"comparison", "arithmetic", "unary"}, args.Mutagens)
	})).Return(nil)

	// Comma-separated and repeatable.
	require.NoError(t, diffRunCmd(t, mockWorkflow, "run", "--mutagens", "comparison,arithmetic", "--mutagens", "unary", "./..."))
}

func TestRunCmd_MutagensDefaultEmpty(t *testing.T) {
	mockWorkflow := domainmocks.NewMockWorkflow(t)
	mockWorkflow.On("Test", mock.Anything, mock.MatchedBy(func(args domain.TestArgs) bool {
		return len(args.Mutagens) == 0 && args.Run == ""
	})).Return(nil)

	require.NoError(t, diffRunCmd(t, mockWorkflow, "run", "./..."))
}

func TestRunCmd_MutagensFromEnv(t *testing.T) {
	t.Setenv("GOOZE_RUN_MUTAGENS", "comparison,logical")

	mockWorkflow := domainmocks.NewMockWorkflow(t)
	mockWorkflow.On("Test", mock.Anything, mock.MatchedBy(func(args domain.TestArgs) bool {
		// The workflow splits on commas, so the raw env value is enough.
		return assert.ObjectsAreEqual([]string{"comparison,logical"}, args.Mutagens)
	})).Return(nil)

	require.NoError(t, diffRunCmd(t, mockWorkflow, "run", "./..."))
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

	mockWorkflow.On("Test", mock.Anything, mock.MatchedBy(func(args domain.TestArgs) bool {
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

	mockWorkflow.On("Test", mock.Anything, mock.MatchedBy(func(args domain.TestArgs) bool {
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

	mockWorkflow.On("Test", mock.Anything, mock.MatchedBy(func(args domain.TestArgs) bool {
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

	mockWorkflow.On("Test", mock.Anything, mock.MatchedBy(func(args domain.TestArgs) bool {
		return args.UseCache == false
	})).Return(nil)

	cmd.SetArgs([]string{"--no-cache", "run", "./..."})
	err := cmd.Execute()
	require.NoError(t, err)

	mockWorkflow.AssertExpectations(t)
}

func TestRunCmd_DefaultsToRecursiveCurrentDir(t *testing.T) {
	mockWorkflow := domainmocks.NewMockWorkflow(t)

	cmd := newRootCmd()
	cmd.AddCommand(newRunCmd())
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	originalWorkflow := workflow
	workflow = mockWorkflow
	defer func() { workflow = originalWorkflow }()

	// No paths given → defaults to ./...
	mockWorkflow.On("Test", mock.Anything, mock.MatchedBy(func(args domain.TestArgs) bool {
		return len(args.Paths) == 1 && args.Paths[0] == m.Path("./...")
	})).Return(nil)

	cmd.SetArgs([]string{"run"})
	err := cmd.Execute()
	require.NoError(t, err)

	mockWorkflow.AssertExpectations(t)
}

func TestRunCmd_EstimateMode(t *testing.T) {
	mockWorkflow := domainmocks.NewMockWorkflow(t)

	cmd := newRootCmd()
	cmd.AddCommand(newRunCmd())
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	originalWorkflow := workflow
	workflow = mockWorkflow
	defer func() { workflow = originalWorkflow }()

	// --estimate routes to Estimate, not Test.
	mockWorkflow.On("Estimate", mock.Anything, mock.MatchedBy(func(args domain.EstimateArgs) bool {
		return len(args.Paths) == 1 && args.Paths[0] == m.Path("./...")
	})).Return(nil)

	cmd.SetArgs([]string{"run", "--estimate", "./..."})
	err := cmd.Execute()
	require.NoError(t, err)

	mockWorkflow.AssertExpectations(t)
	mockWorkflow.AssertNotCalled(t, "Test", mock.Anything, mock.Anything)
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
