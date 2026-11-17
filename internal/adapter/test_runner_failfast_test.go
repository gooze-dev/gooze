package adapter

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gooze.dev/pkg/gooze/internal/domain"
)

// failingModule writes a module whose tests TestA and TestB both fail and
// returns its directory and its test file.
func failingModule(t *testing.T) (string, string) {
	t.Helper()

	dir := t.TempDir()
	testFile := filepath.Join(dir, "p_test.go")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/failing\n\ngo 1.24\n"), 0o600))
	require.NoError(t, os.WriteFile(testFile, []byte(`package p

import "testing"

func TestA(t *testing.T) { t.Fatal("a fails") }

func TestB(t *testing.T) { t.Fatal("b fails") }
`), 0o600))

	return dir, testFile
}

func TestLocalTestRunnerAdapter_RunMutantTestStopsAtTheFirstFailure(t *testing.T) {
	dir, testFile := failingModule(t)
	runner := NewLocalTestRunnerAdapter()

	var _ domain.MutantTestRunner = runner

	output, err := runner.RunMutantTest(context.Background(), dir, testFile, "")
	require.Error(t, err, "a failing test still fails the run")
	assert.Contains(t, output, "--- FAIL: TestA")
	assert.NotContains(t, output, "TestB", "the tests after the first failure don't run")

	output, err = runner.RunGoTest(context.Background(), dir, testFile, "")
	require.Error(t, err)
	assert.Contains(t, output, "--- FAIL: TestA")
	assert.Contains(t, output, "--- FAIL: TestB", "RunGoTest runs every test")
}

func TestLocalTestRunnerAdapter_RunMutantTestKeepsTheTestFlags(t *testing.T) {
	dir, testFile := failingModule(t)
	runner := NewLocalTestRunnerAdapter()
	runner.SetTestFlags([]string{"-run=TestB"})

	output, err := runner.RunMutantTest(context.Background(), dir, testFile, "")
	require.Error(t, err)
	assert.Contains(t, output, "--- FAIL: TestB")
	assert.NotContains(t, output, "TestA")
	assert.Equal(t, []string{"-run=TestB"}, runner.flags, "the flags of the next run are untouched")
	assert.Equal(t, 1, strings.Count(output, "--- FAIL"))
}
