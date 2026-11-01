package adapter

import (
	"bytes"
	"context"
	"os"
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

// RunGoTest runs 'go test -vet=off' for the package that contains testFile,
// with -overlay when overlay is set.
func (a *LocalTestRunnerAdapter) RunGoTest(ctx context.Context, workDir, testFile, overlay string) (string, error) {
	pkgDir := filepath.Dir(testFile)
	if rel, err := filepath.Rel(workDir, pkgDir); err == nil && !filepath.IsAbs(rel) {
		pkgDir = "." + string(filepath.Separator) + rel
	}

	// Vet would reject common mutants (x == a && x == b) before the tests run.
	args := []string{"test", "-count=1", "-vet=off", "-v"}
	// Equivalent: go test treats -overlay "" as no overlay.
	if overlay != "" { //gooze:ignore comparison
		args = append(args, "-overlay", overlay)
	}

	args = append(args, pkgDir)

	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = workDir

	// A killed go test can't remove its work directory, so it goes in one
	// removed here, timed out or not. Removing "" does nothing.
	buildTmp := buildTempDir()
	defer os.RemoveAll(buildTmp) //nolint:errcheck // best effort cleanup.

	cmd.Env = append(os.Environ(), "GOTMPDIR="+buildTmp)
	cmd.WaitDelay = 5 * time.Second
	setProcessGroup(cmd)

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	output := stdout.String() + stderr.String()

	return output, err
}

// buildTempDir creates the directory for one go test's work directory, where
// go itself would put it. When it can't be created it returns "", which leaves
// the choice to go.
func buildTempDir() string {
	dir, err := os.MkdirTemp(os.Getenv("GOTMPDIR"), "gooze-gotest-")
	if err != nil {
		return ""
	}

	return dir
}
