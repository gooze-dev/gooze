package adapter

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"time"
)

// LocalTestRunnerAdapter provides a concrete implementation using os/exec.
type LocalTestRunnerAdapter struct {
}

// NewLocalTestRunnerAdapter constructs a LocalTestRunnerAdapter.
func NewLocalTestRunnerAdapter() *LocalTestRunnerAdapter {
	return &LocalTestRunnerAdapter{}
}

// RunGoTest runs 'go test' for the package that contains testFile, with
// -overlay when overlay is set.
func (a *LocalTestRunnerAdapter) RunGoTest(ctx context.Context, workDir, testFile, overlay string) (string, error) {
	pkgDir := filepath.Dir(testFile)
	if rel, err := filepath.Rel(workDir, pkgDir); err == nil && !filepath.IsAbs(rel) {
		pkgDir = "." + string(filepath.Separator) + rel
	}

	args := []string{"test", "-count=1", "-v"}
	// Equivalent: go test treats -overlay "" as no overlay.
	if overlay != "" { //gooze:ignore comparison
		args = append(args, "-overlay", overlay)
	}

	args = append(args, pkgDir)

	cmd := exec.CommandContext(ctx, "go", args...) //nolint:gosec // pkgDir lies inside the project or its copy; overlay is gooze's own temp file.
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
