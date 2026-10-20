package adapter

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"time"
)

// TestRunnerAdapter abstracts test execution operations for mutation testing.
type TestRunnerAdapter interface {
	// RunGoTest runs 'go test' for the package containing testFile, inside
	// workDir. Returns the combined stdout/stderr output and any error.
	RunGoTest(ctx context.Context, workDir, testFile string) (output string, err error)
}

// LocalTestRunnerAdapter provides a concrete implementation using os/exec.
type LocalTestRunnerAdapter struct {
}

// NewLocalTestRunnerAdapter constructs a LocalTestRunnerAdapter.
func NewLocalTestRunnerAdapter() *LocalTestRunnerAdapter {
	return &LocalTestRunnerAdapter{}
}

// RunGoTest runs 'go test' for the package that contains testFile.
func (a *LocalTestRunnerAdapter) RunGoTest(ctx context.Context, workDir, testFile string) (string, error) {
	pkgDir := filepath.Dir(testFile)
	if rel, err := filepath.Rel(workDir, pkgDir); err == nil && !filepath.IsAbs(rel) {
		pkgDir = "." + string(filepath.Separator) + rel
	}

	cmd := exec.CommandContext(ctx, "go", "test", "-count=1", "-v", pkgDir) //nolint:gosec // pkgDir lies inside the workspace copy.
	cmd.Dir = workDir
	cmd.WaitDelay = 5 * time.Second
	setProcessGroup(cmd)

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	output := stdout.String() + stderr.String()

	return output, err
}
