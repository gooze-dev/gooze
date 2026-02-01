package cmd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"testing"

	m "gooze.dev/pkg/gooze/internal/model"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseShardFlag(t *testing.T) {
	tests := []struct {
		name      string
		shard     string
		wantIndex int
		wantTotal int
	}{
		{"empty string", "", 0, 1},
		{"valid 0/3", "0/3", 0, 3},
		{"valid 1/3", "1/3", 1, 3},
		{"valid 2/3", "2/3", 2, 3},
		{"invalid format", "invalid", 0, 1},
		{"zero total", "0/0", 0, 1},
		{"negative total", "0/-1", 0, 1},
		{"negative index", "-1/3", 0, 1},
		{"index >= total", "3/3", 0, 1},
		{"index > total", "5/3", 0, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotIndex, gotTotal := parseShardFlag(tt.shard)
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

func TestInit(t *testing.T) {
	// Test that init() created all the necessary instances
	assert.NotNil(t, ui)
	assert.NotNil(t, goFileAdapter)
	assert.NotNil(t, soirceFSAdapter)
	assert.NotNil(t, reportStore)
	assert.NotNil(t, fsAdapter)
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
