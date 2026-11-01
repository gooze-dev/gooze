package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/mock"
	"gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
	m "gooze.dev/pkg/gooze/internal/model"
	"gooze.dev/pkg/gooze/pkg/config"
)

// newRootCmd creates a root command with flags configured for testing.
func newRootCmd() *cobra.Command {
	cmd := baseRootCmd()
	configureRootFlags(cmd)
	return cmd
}

func TestParseShardFlag(t *testing.T) {
	tests := []struct {
		name      string
		shard     string
		wantIndex int
		wantTotal int
		wantErr   bool
	}{
		{"empty string", "", 0, 1, false},
		{"valid 0/3", "0/3", 0, 3, false},
		{"valid 1/3", "1/3", 1, 3, false},
		{"valid 2/3", "2/3", 2, 3, false},
		{"invalid format", "invalid", 0, 0, true},
		{"trailing garbage", "1/4abc", 0, 0, true},
		{"zero total", "0/0", 0, 0, true},
		{"negative total", "0/-1", 0, 0, true},
		{"negative index", "-1/3", 0, 0, true},
		{"index >= total", "3/3", 0, 0, true},
		{"index > total", "5/3", 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotIndex, gotTotal, err := parseShardFlag(tt.shard)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, m.ExitUsage, exitCode(context.Background(), err, false))

				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.wantIndex, gotIndex, "index")
			assert.Equal(t, tt.wantTotal, gotTotal, "total")
		})
	}
}

func TestParsePaths(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []m.Path
	}{
		{"empty", []string{}, []m.Path{}},
		{"single", []string{"./..."}, []m.Path{m.Path("./...")}},
		{
			"multiple",
			[]string{"./cmd", "./pkg", "./internal"},
			[]m.Path{m.Path("./cmd"), m.Path("./pkg"), m.Path("./internal")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parsePaths(tt.args)
			require.Len(t, got, len(tt.want))
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewRootCmd(t *testing.T) {
	cmd := newRootCmd()
	assert.Equal(t, "gooze", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.Equal(t, rootLongDescription, cmd.Long)
}

func TestRootCmd_PersistentFlags(t *testing.T) {
	cmd := newRootCmd()
	outputFlag := cmd.PersistentFlags().Lookup("output")
	assert.NotNil(t, outputFlag)
	noCache := cmd.PersistentFlags().Lookup("no-cache")
	assert.NotNil(t, noCache)
	exclude := cmd.PersistentFlags().Lookup("exclude")
	assert.NotNil(t, exclude)
}

func TestRootCmd_HelpOutput(t *testing.T) {
	cmd := newRootCmd()
	output := &bytes.Buffer{}
	cmd.SetOut(output)
	cmd.SetErr(&bytes.Buffer{})

	cmd.SetArgs([]string{})
	err := cmd.Execute()

	require.NoError(t, err)
	assert.Contains(t, output.String(), "Usage:")
	assert.Contains(t, output.String(), "Supports Go-style path patterns")
}

func TestRootCmd_UsageOnlyForArgumentErrors(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantUsage bool
	}{
		{name: "runtime error", args: []string{"fail"}, wantUsage: false},
		{name: "argument error", args: []string{"fail", "extra"}, wantUsage: true},
		{name: "flag error", args: []string{"fail", "--nope"}, wantUsage: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newRootCmd()
			cmd.AddCommand(&cobra.Command{
				Use:  "fail",
				Args: cobra.NoArgs,
				RunE: func(*cobra.Command, []string) error {
					return fmt.Errorf("runtime failure")
				},
			})

			output := &bytes.Buffer{}
			cmd.SetOut(output)
			cmd.SetErr(output)
			cmd.SetArgs(append(tt.args, "--log-output", t.TempDir()+"/gooze.log"))

			require.Error(t, cmd.Execute())
			assert.Equal(t, tt.wantUsage, bytes.Contains(output.Bytes(), []byte("Usage:")), output.String())
		})
	}
}

func TestInit(t *testing.T) {
	// Test that init() created all the necessary instances
	assert.NotNil(t, ui)
	assert.NotNil(t, goFileAdapter)
	assert.NotNil(t, sourceFSAdapter)
	assert.NotNil(t, reportStore)
	assert.NotNil(t, testAdapter)
	assert.NotNil(t, orchestrator)
	assert.NotNil(t, mutagen)
	assert.NotNil(t, workflow)
}

func TestExecute(t *testing.T) {
	// Save original rootCmd
	originalRootCmd := rootCmd

	// Create a mock command that succeeds
	mockCmd := &cobra.Command{
		Use: "test",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	mockCmd.SetOut(&bytes.Buffer{})
	mockCmd.SetErr(&bytes.Buffer{})

	rootCmd = mockCmd

	// Execute should not panic or exit
	// We can't easily test os.Exit, but we can verify no error path
	Execute()

	// Restore
	rootCmd = originalRootCmd
}

func TestExecute_WithError(t *testing.T) {
	// Save original rootCmd
	originalRootCmd := rootCmd
	defer func() {
		rootCmd = originalRootCmd
	}()

	// Create a mock command that fails
	mockCmd := &cobra.Command{
		Use: "test",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("command failed")
		},
	}
	mockCmd.SetOut(&bytes.Buffer{})
	mockCmd.SetErr(&bytes.Buffer{})

	rootCmd = mockCmd

	// This will cause os.Exit(1) to be called, which we can't intercept
	// So we just verify the command itself errors
	err := rootCmd.Execute()
	require.Error(t, err)
}

func TestExecute_ProcessLevel_Success(t *testing.T) {
	if os.Getenv("TEST_EXECUTE_SUBPROCESS") == "1" {
		// This runs in the subprocess
		// Mock successful command
		originalRootCmd := rootCmd
		mockCmd := &cobra.Command{
			Use: "test",
			RunE: func(cmd *cobra.Command, args []string) error {
				fmt.Println("success")
				return nil
			},
		}
		mockCmd.SetOut(os.Stdout)
		mockCmd.SetErr(os.Stderr)
		rootCmd = mockCmd
		defer func() { rootCmd = originalRootCmd }()

		Execute()
		return
	}

	// Parent process: spawn subprocess
	cmd := exec.Command(os.Args[0], "-test.run=TestExecute_ProcessLevel_Success")
	cmd.Env = append(os.Environ(), "TEST_EXECUTE_SUBPROCESS=1")
	output, err := cmd.CombinedOutput()

	require.NoError(t, err, "output: %s", output)
	assert.Contains(t, string(output), "success")

	if exitErr, ok := err.(*exec.ExitError); ok {
		assert.Equal(t, 0, exitErr.ExitCode())
	}
}

func TestExecute_ProcessLevel_Failure(t *testing.T) {
	if os.Getenv("TEST_EXECUTE_SUBPROCESS_FAIL") == "1" {
		// This runs in the subprocess
		// Mock failing command
		originalRootCmd := rootCmd
		mockCmd := &cobra.Command{
			Use: "test",
			PersistentPreRun: func(cmd *cobra.Command, _ []string) {
				cmd.Root().SilenceUsage = true
			},
			RunE: func(cmd *cobra.Command, args []string) error {
				fmt.Fprintln(os.Stderr, "error occurred")
				return fmt.Errorf("command failed")
			},
		}
		mockCmd.SetOut(os.Stdout)
		mockCmd.SetErr(os.Stderr)
		rootCmd = mockCmd
		defer func() { rootCmd = originalRootCmd }()

		Execute() // This should call os.Exit(1)
		return
	}

	// Parent process: spawn subprocess
	cmd := exec.Command(os.Args[0], "-test.run=TestExecute_ProcessLevel_Failure")
	cmd.Env = append(os.Environ(), "TEST_EXECUTE_SUBPROCESS_FAIL=1")
	output, err := cmd.CombinedOutput()

	require.Error(t, err)

	if exitErr, ok := err.(*exec.ExitError); ok {
		assert.Equal(t, 1, exitErr.ExitCode())
	} else {
		assert.Fail(t, "expected exec.ExitError", "got %T", err)
	}

	assert.Contains(t, string(output), "error occurred")
}

// runExitSubprocess runs the real Execute in a subprocess on args, with
// rootCmd's run returning runErr, and returns the subprocess's exit code.
func runExitSubprocess(t *testing.T, mode string, args ...string) int {
	t.Helper()

	cmd := exec.Command(os.Args[0], append([]string{"-test.run=TestExecute_ProcessLevel_ExitCodes", "--"}, args...)...)
	cmd.Env = append(os.Environ(), "TEST_EXECUTE_EXIT_MODE="+mode)
	output, err := cmd.CombinedOutput()

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		require.NoError(t, err, "output: %s", output)
		return 0
	}

	return exitErr.ExitCode()
}

func TestExecute_ProcessLevel_ExitCodes(t *testing.T) {
	if mode := os.Getenv("TEST_EXECUTE_EXIT_MODE"); mode != "" {
		mockCmd := &cobra.Command{
			Use: "test",
			PersistentPreRun: func(cmd *cobra.Command, _ []string) {
				cmd.Root().SilenceUsage = true
			},
			RunE: func(*cobra.Command, []string) error {
				if mode == "threshold" {
					return domain.WithExitCode(m.ExitBelowThreshold, errors.New("below threshold"))
				}

				return nil
			},
		}
		mockCmd.Flags().Bool("known", false, "")
		mockCmd.SetArgs(os.Args[slices.Index(os.Args, "--")+1:])
		rootCmd = mockCmd

		Execute()

		return
	}

	assert.Equal(t, 0, runExitSubprocess(t, "ok"))
	assert.Equal(t, 2, runExitSubprocess(t, "ok", "--unknown"))
	assert.Equal(t, 3, runExitSubprocess(t, "threshold", "--known"))
}

func TestRootCmd_FlagDefaultsMatchSchema(t *testing.T) {
	t.Setenv("GOOZE_OUTPUT", "env-reports")
	t.Setenv("GOOZE_NO_CACHE", "true")
	t.Setenv("GOOZE_NO_UPDATE_CHECK", "true")
	t.Setenv("GOOZE_PATHS_EXCLUDE", "env-exclude")

	for _, cmd := range []*cobra.Command{rootCmd, newRootCmd()} {
		assert.Equal(t, ".gooze-reports", cmd.PersistentFlags().Lookup(outputFlagName).DefValue)
		assert.Equal(t, "false", cmd.PersistentFlags().Lookup(noCacheFlagName).DefValue)
		assert.Equal(t, "false", cmd.PersistentFlags().Lookup(noUpdateCheckFlag).DefValue)
		assert.Equal(t, "[]", cmd.PersistentFlags().Lookup(excludeFlagName).DefValue)
	}
}

// executeRun runs gooze run against a mock workflow and returns the error and
// the log level the run saw.
func executeRun(t *testing.T, mockCalled bool, args ...string) (string, error) {
	t.Helper()

	mockWorkflow := domainmocks.NewMockWorkflow(t)
	if mockCalled {
		mockWorkflow.On("Test", mock.Anything, mock.Anything).Return(nil)
	}

	originalWorkflow := workflow
	workflow = mockWorkflow

	t.Cleanup(func() {
		workflow = originalWorkflow

		newRootCmd() // Rebind the keys to fresh, unset flags for later tests.
		newRunCmd()
	})

	cmd := newRootCmd()
	cmd.AddCommand(newRunCmd())
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(append([]string{"run"}, args...))

	err := cmd.Execute()

	return viper.GetString(config.KeyLogLevel), err
}

func TestLogLevelFlag_SetsTheLevel(t *testing.T) {
	level, err := executeRun(t, true, "--log-level", "warn", "./...")
	require.NoError(t, err)
	assert.Equal(t, "warn", level)
}

func TestLogLevelFlag_WinsOverTheEnvironment(t *testing.T) {
	t.Setenv("GOOZE_LOG_LEVEL", "error")

	level, err := executeRun(t, true, "--log-level", "debug", "./...")
	require.NoError(t, err)
	assert.Equal(t, "debug", level)
}

func TestLogLevelFlag_UnsetLeavesTheEnvironment(t *testing.T) {
	t.Setenv("GOOZE_LOG_LEVEL", "error")

	level, err := executeRun(t, true, "./...")
	require.NoError(t, err)
	assert.Equal(t, "error", level)
}

func TestLogLevelFlag_RejectsAnUnknownLevel(t *testing.T) {
	_, err := executeRun(t, false, "--log-level", "loud", "./...")
	require.ErrorContains(t, err, "invalid log.level")
}

func TestRootCmd_VerboseSetsDebugLogging(t *testing.T) {
	for name, tt := range map[string]struct {
		args []string
		env  string
		want bool
	}{
		"off":  {},
		"flag": {args: []string{"--verbose"}, want: true},
		"env":  {env: "true", want: true},
	} {
		t.Run(name, func(t *testing.T) {
			if tt.env != "" {
				t.Setenv("GOOZE_LOG_VERBOSE", tt.env)
			}

			original := slog.Default()
			t.Cleanup(func() { slog.SetDefault(original) })

			args := append([]string{"--log-output", filepath.Join(t.TempDir(), "gooze.log"), "./..."}, tt.args...)
			_, err := executeRun(t, true, args...)
			require.NoError(t, err)
			assert.Equal(t, tt.want, slog.Default().Enabled(context.Background(), slog.LevelDebug))
		})
	}
}

func TestRootCmd_LogOutputWinsOverTheConfig(t *testing.T) {
	dir := t.TempDir()
	given, configured := filepath.Join(dir, "given.log"), filepath.Join(dir, "configured.log")
	t.Setenv("GOOZE_LOG_FILENAME", configured)

	original := slog.Default()
	t.Cleanup(func() { slog.SetDefault(original) })

	_, err := executeRun(t, true, "--log-output", given, "./...")
	require.NoError(t, err)

	slog.Info("logged here")
	assert.FileExists(t, given)
	assert.NoFileExists(t, configured)
}
