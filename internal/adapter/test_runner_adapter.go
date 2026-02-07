package adapter

import (
	"bytes"
	"context"
	"os/exec"
)

// TestRunnerAdapter abstracts test execution operations for mutation testing.
type TestRunnerAdapter interface {
	// RunGoTest runs 'go test' on a specific test file in the given directory.
	// Returns the combined stdout/stderr output and any error.
	RunGoTest(ctx context.Context, workDir, testFile string) (output string, err error)
}

// LocalTestRunnerAdapter provides a concrete implementation using os/exec.
type LocalTestRunnerAdapter struct {
}

// NewLocalTestRunnerAdapter constructs a LocalTestRunnerAdapter with default 30s timeout.
func NewLocalTestRunnerAdapter() *LocalTestRunnerAdapter {
	return &LocalTestRunnerAdapter{}
}

// RunGoTest runs 'go test' on a specific test file in the given directory.
func (a *LocalTestRunnerAdapter) RunGoTest(ctx context.Context, workDir, testFile string) (string, error) {
	cmd := exec.CommandContext(ctx, "go", "test", "-v", testFile)
	cmd.Dir = workDir

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	output := stdout.String() + stderr.String()

	return output, err
}
