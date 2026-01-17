package cmd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	controllermocks "github.com/mouse-blink/gooze/internal/controller/mocks"
	"github.com/mouse-blink/gooze/internal/domain"
	domainmocks "github.com/mouse-blink/gooze/internal/domain/mocks"
	m "github.com/mouse-blink/gooze/internal/model"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/mock"
)

func TestRootCmd_ListFlag(t *testing.T) {
	// Setup mocks
	mockWorkflow := domainmocks.NewMockWorkflow(t)
	mockUI := controllermocks.NewMockUI(t)

	// Create a new root command for testing
	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	// Override the global workflow
	originalWorkflow := workflow
	workflow = mockWorkflow
	defer func() { workflow = originalWorkflow }()

	// Set expectations
	mockWorkflow.On("Estimate", mock.MatchedBy(func(args domain.EstimateArgs) bool {
		return args.UseCache == true
	})).Return(nil)

	// Execute command with --list flag
	cmd.SetArgs([]string{"--list", "./..."})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	mockWorkflow.AssertExpectations(t)
	mockUI.AssertExpectations(t)
}

func TestRootCmd_TestMode(t *testing.T) {
	// Setup mocks
	mockWorkflow := domainmocks.NewMockWorkflow(t)

	// Create a new root command for testing
	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	// Override the global workflow
	originalWorkflow := workflow
	workflow = mockWorkflow
	defer func() { workflow = originalWorkflow }()

	// Set expectations
	mockWorkflow.On("Test", mock.MatchedBy(func(args domain.TestArgs) bool {
		return args.Threads == 2 &&
			args.ShardIndex == 0 &&
			args.TotalShardCount == 1 &&
			args.Reports == m.Path(".gooze-reports")
	})).Return(nil)

	// Execute command without --list flag
	cmd.SetArgs([]string{"--parallel", "2", "./..."})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	mockWorkflow.AssertExpectations(t)
}

func TestRootCmd_WithSharding(t *testing.T) {
	// Setup mocks
	mockWorkflow := domainmocks.NewMockWorkflow(t)

	// Create a new root command for testing
	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	// Override the global workflow
	originalWorkflow := workflow
	workflow = mockWorkflow
	defer func() { workflow = originalWorkflow }()

	// Set expectations for shard 1/3
	mockWorkflow.On("Test", mock.MatchedBy(func(args domain.TestArgs) bool {
		return args.ShardIndex == 1 && args.TotalShardCount == 3
	})).Return(nil)

	// Execute command with sharding
	cmd.SetArgs([]string{"--shard", "1/3", "./..."})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	mockWorkflow.AssertExpectations(t)
}

func TestRootCmd_MultiplePaths(t *testing.T) {
	// Setup mocks
	mockWorkflow := domainmocks.NewMockWorkflow(t)

	// Create a new root command for testing
	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	// Override the global workflow
	originalWorkflow := workflow
	workflow = mockWorkflow
	defer func() { workflow = originalWorkflow }()

	// Set expectations
	mockWorkflow.On("Test", mock.MatchedBy(func(args domain.TestArgs) bool {
		return len(args.Paths) == 3 &&
			args.Paths[0] == m.Path("./cmd") &&
			args.Paths[1] == m.Path("./pkg") &&
			args.Paths[2] == m.Path("./internal")
	})).Return(nil)

	// Execute command with multiple paths
	cmd.SetArgs([]string{"./cmd", "./pkg", "./internal"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	mockWorkflow.AssertExpectations(t)
}

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
			if gotIndex != tt.wantIndex {
				t.Errorf("parseShardFlag() index = %v, want %v", gotIndex, tt.wantIndex)
			}
			if gotTotal != tt.wantTotal {
				t.Errorf("parseShardFlag() total = %v, want %v", gotTotal, tt.wantTotal)
			}
		})
	}
}

func TestNewRootCmd(t *testing.T) {
	cmd := newRootCmd()
	if cmd.Use != "gooze [paths...]" {
		t.Errorf("newRootCmd() Use = %v, want %v", cmd.Use, "gooze [paths...]")
	}
	if cmd.Short == "" {
		t.Error("newRootCmd() Short should not be empty")
	}
	if cmd.Long == "" {
		t.Error("newRootCmd() Long should not be empty")
	}

	// Check flags
	listFlag := cmd.Flags().Lookup("list")
	if listFlag == nil {
		t.Error("newRootCmd() missing --list flag")
	}
	parallelFlag := cmd.Flags().Lookup("parallel")
	if parallelFlag == nil {
		t.Error("newRootCmd() missing --parallel flag")
	}
	shardFlag := cmd.Flags().Lookup("shard")
	if shardFlag == nil {
		t.Error("newRootCmd() missing --shard flag")
	}
}

func TestInit(t *testing.T) {
	// Test that init() created all the necessary instances
	if ui == nil {
		t.Error("init() ui is nil")
	}
	if goFileAdapter == nil {
		t.Error("init() goFileAdapter is nil")
	}
	if soirceFSAdapter == nil {
		t.Error("init() soirceFSAdapter is nil")
	}
	if reportStore == nil {
		t.Error("init() reportStore is nil")
	}
	if fsAdapter == nil {
		t.Error("init() fsAdapter is nil")
	}
	if testAdapter == nil {
		t.Error("init() testAdapter is nil")
	}
	if orchestrator == nil {
		t.Error("init() orchestrator is nil")
	}
	if mutagen == nil {
		t.Error("init() mutagen is nil")
	}
	if workflow == nil {
		t.Error("init() workflow is nil")
	}
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
	if err == nil {
		t.Error("Expected command to return an error")
	}
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

	if err != nil {
		t.Errorf("Process exited with error: %v, output: %s", err, output)
	}

	if !strings.Contains(string(output), "success") {
		t.Errorf("Expected 'success' in output, got: %s", output)
	}

	if exitErr, ok := err.(*exec.ExitError); ok {
		if exitErr.ExitCode() != 0 {
			t.Errorf("Expected exit code 0, got %d", exitErr.ExitCode())
		}
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

	if err == nil {
		t.Error("Expected process to exit with error")
	}

	if exitErr, ok := err.(*exec.ExitError); ok {
		if exitErr.ExitCode() != 1 {
			t.Errorf("Expected exit code 1, got %d", exitErr.ExitCode())
		}
	} else {
		t.Errorf("Expected exec.ExitError, got %T", err)
	}

	if !strings.Contains(string(output), "error occurred") {
		t.Logf("Output: %s", output)
	}
}
