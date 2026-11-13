package adapter

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"gooze.dev/pkg/gooze/internal/domain"
)

// LocalTestRunnerAdapter provides a concrete implementation using os/exec.
type LocalTestRunnerAdapter struct {
	env         []string
	flags       []string
	memoryLimit int64
}

// NewLocalTestRunnerAdapter constructs a LocalTestRunnerAdapter.
func NewLocalTestRunnerAdapter() *LocalTestRunnerAdapter {
	return &LocalTestRunnerAdapter{}
}

// SetBuildEnv adds env to the environment of the go commands it runs.
func (a *LocalTestRunnerAdapter) SetBuildEnv(env []string) {
	a.env = env
}

// SetTestFlags adds flags to the go test commands it runs.
func (a *LocalTestRunnerAdapter) SetTestFlags(flags []string) {
	a.flags = flags
}

// SetMemoryLimit sets the memory, in bytes, the processes of one go test may
// hold together; 0 is no limit. It is enforced on linux only.
func (a *LocalTestRunnerAdapter) SetMemoryLimit(limit int64) {
	a.memoryLimit = limit
}

// RunGoTest runs 'go test -vet=off' for the package that contains testFile,
// with -overlay when overlay is set. It kills the test and returns a
// domain.ErrMemoryLimit when its processes hold more than the memory limit.
// Of long output it returns the start and the end of each stream. On unix
// the tests get a temp directory of their own, removed when they end.
func (a *LocalTestRunnerAdapter) RunGoTest(ctx context.Context, workDir, testFile, overlay string) (string, error) {
	cmd := newGoTestCommand(ctx, workDir, testFile, overlay, a.flags)

	// A killed go test can't remove its work directory, so it goes in one
	// removed here, timed out or not. Removing "" does nothing.
	buildTmp := buildTempDir()
	defer os.RemoveAll(buildTmp) //nolint:errcheck // best effort cleanup.

	// A killed test doesn't remove its temp files either. An empty TMPDIR
	// is the system's temp directory.
	testTmp := testTempDir()
	defer os.RemoveAll(testTmp) //nolint:errcheck // best effort cleanup.

	cmd.Env = append(append(os.Environ(), "GOTMPDIR="+buildTmp, "TMPDIR="+testTmp), a.env...)

	var stdout, stderr outputBuffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return "", err
	}

	overLimit := watchMemory(cmd, a.memoryLimit)
	err := cmd.Wait()

	output := stdout.String() + stderr.String()

	if overLimit() {
		return output, fmt.Errorf("go test held more than %d MiB: %w", a.memoryLimit>>20, domain.ErrMemoryLimit)
	}

	return output, err
}

// newGoTestCommand returns the go test command for the package of testFile,
// with the user's flags after gooze's, which a canceled ctx kills along with
// its children.
func newGoTestCommand(ctx context.Context, workDir, testFile, overlay string, flags []string) *exec.Cmd {
	// Vet would reject common mutants (x == a && x == b) before the tests run.
	args := []string{"test", "-count=1", "-vet=off", "-v"}
	// Equivalent: go test treats -overlay "" as no overlay.
	if overlay != "" { //gooze:ignore comparison
		args = append(args, "-overlay", overlay)
	}

	args = append(append(args, flags...), packageArg(workDir, testFile))

	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = workDir
	cmd.WaitDelay = 5 * time.Second
	setProcessGroup(cmd)

	return cmd
}

// packageArg returns go test's argument for the package of testFile: relative
// to workDir, when it can be.
func packageArg(workDir, testFile string) string {
	pkgDir := filepath.Dir(testFile)
	if rel, err := filepath.Rel(workDir, pkgDir); err == nil && !filepath.IsAbs(rel) {
		return "." + string(filepath.Separator) + rel
	}

	return pkgDir
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

// testTempDir creates the directory the tests of one go test get as their
// temp directory, in the system's. When it can't be created it returns "",
// which leaves them the system's.
func testTempDir() string {
	// A short name: the paths of unix sockets under it have little room.
	dir, err := os.MkdirTemp("", "gz")
	if err != nil {
		return ""
	}

	return dir
}
