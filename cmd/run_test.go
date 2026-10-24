package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
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

func TestRunCmd_WorkspaceFlag(t *testing.T) {
	for _, tt := range []struct {
		args []string
		want domain.WorkspaceMode
	}{
		{args: []string{"run", "./..."}, want: domain.WorkspaceOverlay},
		{args: []string{"run", "--workspace", "deep-copy", "./..."}, want: domain.WorkspaceDeepCopy},
	} {
		t.Run(string(tt.want), func(t *testing.T) {
			mockWorkflow := domainmocks.NewMockWorkflow(t)

			cmd := newRootCmd()
			cmd.AddCommand(newRunCmd())
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			originalWorkflow := workflow
			workflow = mockWorkflow
			defer func() { workflow = originalWorkflow }()
			// Rebind run.workspace to a fresh, unset flag so later tests see the default.
			defer newRunCmd()

			mockWorkflow.On("Test", mock.Anything, mock.MatchedBy(func(args domain.TestArgs) bool {
				return args.Workspace == tt.want
			})).Return(nil)

			cmd.SetArgs(tt.args)
			require.NoError(t, cmd.Execute())
			mockWorkflow.AssertExpectations(t)
		})
	}
}

func TestRunCmd_FailFastFlag(t *testing.T) {
	for _, tt := range []struct {
		args []string
		env  string
		want domain.FailFast
	}{
		{args: []string{"run", "./..."}, want: domain.FailFastOff},
		{args: []string{"run", "--fail-fast", "gate", "./..."}, want: domain.FailFastGate},
		{args: []string{"run", "--fail-fast", "first", "./..."}, want: domain.FailFastFirst},
		{args: []string{"run", "./..."}, env: "file", want: domain.FailFastFile},
	} {
		t.Run(string(tt.want), func(t *testing.T) {
			if tt.env != "" {
				t.Setenv("GOOZE_RUN_FAIL_FAST", tt.env)
			}

			mockWorkflow := domainmocks.NewMockWorkflow(t)

			cmd := newRootCmd()
			cmd.AddCommand(newRunCmd())
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			originalWorkflow := workflow
			workflow = mockWorkflow
			defer func() { workflow = originalWorkflow }()
			// Rebind run.fail_fast to a fresh, unset flag so later tests see the default.
			defer newRunCmd()

			mockWorkflow.On("Test", mock.Anything, mock.MatchedBy(func(args domain.TestArgs) bool {
				return args.FailFast == tt.want
			})).Return(nil)

			cmd.SetArgs(tt.args)
			require.NoError(t, cmd.Execute())
			mockWorkflow.AssertExpectations(t)
		})
	}
}

func TestRunCmd_RejectsUnknownFailFast(t *testing.T) {
	mockWorkflow := domainmocks.NewMockWorkflow(t)

	cmd := newRootCmd()
	cmd.AddCommand(newRunCmd())
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	originalWorkflow := workflow
	workflow = mockWorkflow
	defer func() { workflow = originalWorkflow }()
	defer newRunCmd()

	cmd.SetArgs([]string{"run", "--fail-fast", "always", "./..."})
	err := cmd.Execute()
	require.EqualError(t, err, `invalid run.fail_fast: "always" is not one of off, gate, file, first`)
	assert.Equal(t, m.ExitUsage, exitCode(context.Background(), err, false))
}

func TestRunCmd_RejectsUnknownWorkspace(t *testing.T) {
	mockWorkflow := domainmocks.NewMockWorkflow(t)

	cmd := newRootCmd()
	cmd.AddCommand(newRunCmd())
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	originalWorkflow := workflow
	workflow = mockWorkflow
	defer func() { workflow = originalWorkflow }()
	defer newRunCmd()

	cmd.SetArgs([]string{"run", "--workspace", "overly", "./..."})
	err := cmd.Execute()
	require.EqualError(t, err, `invalid run.workspace: "overly" is not one of overlay, deep-copy`)
	assert.Equal(t, m.ExitUsage, exitCode(context.Background(), err, false))
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

func TestRunCmd_RunWithShardIsRejected(t *testing.T) {
	mockWorkflow := domainmocks.NewMockWorkflow(t)

	err := diffRunCmd(t, mockWorkflow, "run", "--run", "^InRange$", "--shard", "0/2", "./...")
	require.EqualError(t, err, "--run can't be combined with --shard: a function-level run is small, so run it in one job")
	assert.Equal(t, m.ExitUsage, exitCode(context.Background(), err, false))
	mockWorkflow.AssertNotCalled(t, "Test", mock.Anything, mock.Anything)
}

func TestRunCmd_DiffWithShardIsAllowed(t *testing.T) {
	mockWorkflow := domainmocks.NewMockWorkflow(t)
	mockWorkflow.On("Test", mock.Anything, mock.MatchedBy(func(args domain.TestArgs) bool {
		return args.Diff == "main" && args.TotalShardCount == 2
	})).Return(nil)

	require.NoError(t, diffRunCmd(t, mockWorkflow, "run", "--diff", "main", "--shard", "1/2", "./..."))
}

// thresholdsOfRun runs gooze run with args and returns the thresholds it
// passes to the workflow.
func thresholdsOfRun(t *testing.T, args ...string) domain.Thresholds {
	t.Helper()

	var got domain.Thresholds

	mockWorkflow := domainmocks.NewMockWorkflow(t)
	mockWorkflow.On("Test", mock.Anything, mock.Anything).Run(func(a mock.Arguments) {
		got = a.Get(1).(domain.TestArgs).Thresholds
	}).Return(nil)

	require.NoError(t, diffRunCmd(t, mockWorkflow, append([]string{"run"}, args...)...))

	return got
}

func TestRunCmd_Thresholds(t *testing.T) {
	assert.Equal(t, domain.Thresholds{Key: "threshold"}, thresholdsOfRun(t, "./..."))
	assert.Equal(t, domain.Thresholds{Key: "threshold", Total: 80, File: 50.5},
		thresholdsOfRun(t, "--threshold-total", "80", "--threshold-file", "50.5", "--threshold-diff-total", "99", "./..."),
		"a run without --diff ignores the diff thresholds")
}

func TestRunCmd_DiffThresholdsReplaceThePlainOnes(t *testing.T) {
	assert.Equal(t, domain.Thresholds{Key: "threshold.diff", Average: 90},
		thresholdsOfRun(t, "--diff", "main", "--threshold-total", "80", "--threshold-diff-average", "90", "./..."))
}

func TestRunCmd_MaxSurvivors(t *testing.T) {
	assert.Equal(t, domain.Thresholds{Key: "threshold", Total: 80, MaxSurvivors: 5},
		thresholdsOfRun(t, "--threshold-total", "80", "--threshold-max-survivors", "5", "./..."))
	assert.Equal(t, domain.Thresholds{Key: "threshold.diff", MaxSurvivors: 5},
		thresholdsOfRun(t, "--diff", "main", "--threshold-max-survivors", "5", "./..."))
}

func TestRunCmd_MaxSurvivorsFromEnv(t *testing.T) {
	t.Setenv("GOOZE_THRESHOLD_MAX_SURVIVORS", "3")

	assert.Equal(t, domain.Thresholds{Key: "threshold", MaxSurvivors: 3}, thresholdsOfRun(t, "./..."))
	assert.Equal(t, domain.Thresholds{Key: "threshold.diff", MaxSurvivors: 3}, thresholdsOfRun(t, "--diff", "main", "./..."))
}

func TestRunCmd_ThresholdsFromEnv(t *testing.T) {
	t.Setenv("GOOZE_THRESHOLD_TOTAL", "70")
	t.Setenv("GOOZE_THRESHOLD_DIFF_FILE", "85.5")

	assert.Equal(t, domain.Thresholds{Key: "threshold", Total: 70}, thresholdsOfRun(t, "./..."))
	assert.Equal(t, domain.Thresholds{Key: "threshold.diff", File: 85.5}, thresholdsOfRun(t, "--diff", "main", "./..."))
}

func TestRunCmd_BadThresholdIsAUsageError(t *testing.T) {
	mockWorkflow := domainmocks.NewMockWorkflow(t)

	err := diffRunCmd(t, mockWorkflow, "run", "--threshold-diff-total", "120", "./...")
	require.EqualError(t, err, "invalid --threshold-diff-total: 120 is out of range: must be between 0 and 100")

	err = diffRunCmd(t, mockWorkflow, "run", "--threshold-max-survivors", "-2", "./...")
	require.EqualError(t, err, "invalid --threshold-max-survivors: -2 is out of range: must be at least 0")
	assert.Equal(t, m.ExitUsage, exitCode(context.Background(), err, false))
	mockWorkflow.AssertNotCalled(t, "Test", mock.Anything, mock.Anything)
}

func TestRunCmd_FlagDefaultsMatchSchema(t *testing.T) {
	t.Setenv("GOOZE_RUN_WORKSPACE", "deep-copy")
	t.Setenv("GOOZE_RUN_DIFF", "origin/main")
	t.Setenv("GOOZE_RUN_MUTAGENS", "comparison")
	t.Setenv("GOOZE_RUN_FAIL_FAST", "gate")

	for _, cmd := range []*cobra.Command{runCmd, newRunCmd()} {
		assert.Equal(t, "1", cmd.Flags().Lookup(runParallelFlagName).DefValue)
		assert.Equal(t, "120", cmd.Flags().Lookup(mutationTimeoutFlagName).DefValue)
		assert.Empty(t, cmd.Flags().Lookup(coverageProfileFlagName).DefValue)
		assert.Empty(t, cmd.Flags().Lookup(diffFlagName).DefValue)
		assert.Equal(t, "[]", cmd.Flags().Lookup(mutagensFlagName).DefValue)
		assert.Equal(t, "overlay", cmd.Flags().Lookup(workspaceFlagName).DefValue)
		assert.Equal(t, "off", cmd.Flags().Lookup(failFastFlagName).DefValue)
	}
}

func TestRunCmd_HelpShowsSchemaDefaults(t *testing.T) {
	t.Setenv("GOOZE_RUN_PARALLEL", "7")
	t.Setenv("GOOZE_RUN_MUTATION_TIMEOUT", "9")
	t.Setenv("GOOZE_RUN_COVERAGE_PROFILE", "env.out")
	t.Setenv("GOOZE_OUTPUT", "env-reports")

	cmd := newRootCmd()
	cmd.AddCommand(newRunCmd())

	output := &bytes.Buffer{}
	cmd.SetOut(output)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"run", "--help"})
	require.NoError(t, cmd.Execute())

	assert.Regexp(t, `--parallel int\s+number of parallel workers for mutation testing \(default 1\)`, output.String())
	assert.Regexp(t, `--mutation-timeout int\s+timeout duration for testing a mutation in seconds \(default 120\)`, output.String())
	assert.Regexp(t, `--workspace string\s+how mutants are built: .*\(default "overlay"\)`, output.String())
	assert.Regexp(t, `--output string\s+output directory for mutation testing reports \(default "\.gooze-reports"\)`, output.String())
	assert.NotContains(t, output.String(), "env.out")
	assert.NotContains(t, output.String(), "env-reports")
}

func TestRunCmd_MutationTimeoutEnvIsDecimal(t *testing.T) {
	t.Setenv("GOOZE_RUN_MUTATION_TIMEOUT", "090")
	t.Cleanup(func() { newRunCmd() })

	mockWorkflow := domainmocks.NewMockWorkflow(t)

	cmd := newRootCmd()
	cmd.AddCommand(newRunCmd())
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	originalWorkflow := workflow
	workflow = mockWorkflow
	defer func() { workflow = originalWorkflow }()

	mockWorkflow.On("Test", mock.Anything, mock.MatchedBy(func(args domain.TestArgs) bool {
		return args.MutationTimeout == 90*time.Second
	})).Return(nil)

	cmd.SetArgs([]string{"run", "./..."})
	require.NoError(t, cmd.Execute())

	mockWorkflow.AssertExpectations(t)
}

func TestRunCmd_ParallelPrecedence(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		env  string
		args []string
		want int
	}{
		{"default", "", "", nil, 1},
		{"yaml over default", "run:\n  parallel: 3\n", "", nil, 3},
		{"env over yaml", "run:\n  parallel: 3\n", "5", nil, 5},
		{"flag over env", "run:\n  parallel: 3\n", "5", []string{"--parallel", "7"}, 7},
		{"env with a leading zero is decimal", "", "010", nil, 10},
		{"yaml string with a leading zero is decimal", "run:\n  parallel: \"010\"\n", "", nil, 10},
	}

	t.Cleanup(func() { newRunCmd() })

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, viper.ReadConfig(strings.NewReader(tt.yaml)))
			defer func() { require.NoError(t, viper.ReadConfig(strings.NewReader(""))) }()

			if tt.env != "" {
				t.Setenv("GOOZE_RUN_PARALLEL", tt.env)
			}

			mockWorkflow := domainmocks.NewMockWorkflow(t)

			cmd := newRootCmd()
			cmd.AddCommand(newRunCmd())
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			originalWorkflow := workflow
			workflow = mockWorkflow
			defer func() { workflow = originalWorkflow }()

			mockWorkflow.On("Test", mock.Anything, mock.MatchedBy(func(args domain.TestArgs) bool {
				return args.Threads == tt.want
			})).Return(nil)

			cmd.SetArgs(append(append([]string{"run"}, tt.args...), "./..."))
			require.NoError(t, cmd.Execute())

			mockWorkflow.AssertExpectations(t)
		})
	}
}

func TestParseShardFlag_Bounds(t *testing.T) {
	index, total, err := parseShardFlag("0/1")
	require.NoError(t, err)
	assert.Equal(t, [2]int{0, 1}, [2]int{index, total}, "one shard is valid")

	_, _, err = parseShardFlag("01/3")
	require.EqualError(t, err, `invalid --shard "01/3": expected INDEX/TOTAL (e.g. 0/3)`, "numbers are written as gooze prints them")
}
