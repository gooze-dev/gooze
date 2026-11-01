package adapter

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sleepingModule writes a module whose test never finishes in time.
func sleepingModule(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	files := map[string]string{
		"go.mod":    "module sleeping\n\ngo 1.21\n",
		"a_test.go": "package sleeping\n\nimport (\n\t\"testing\"\n\t\"time\"\n)\n\nfunc TestSleep(t *testing.T) { time.Sleep(time.Minute) }\n",
	}

	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
	}

	return dir
}

func TestLocalTestRunnerAdapter_RunGoTest_TimeoutLeavesNothingInTempDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	t.Setenv("GOTMPDIR", "")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := NewLocalTestRunnerAdapter().RunGoTest(ctx, sleepingModule(t), "./...", "")
	require.Error(t, err)

	entries, err := os.ReadDir(tmp)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestLocalTestRunnerAdapter_RunGoTest_TimeoutLeavesNothingInGOTMPDIR(t *testing.T) {
	tmp, goTmp := t.TempDir(), t.TempDir()
	t.Setenv("TMPDIR", tmp)
	t.Setenv("GOTMPDIR", goTmp)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := NewLocalTestRunnerAdapter().RunGoTest(ctx, sleepingModule(t), "./...", "")
	require.Error(t, err)

	for _, dir := range []string{tmp, goTmp} {
		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		assert.Empty(t, entries, dir)
	}
}

func TestBuildTempDir_IsUnderGOTMPDIRWhenSet(t *testing.T) {
	goTmp := t.TempDir()
	t.Setenv("GOTMPDIR", goTmp)

	dir := buildTempDir()

	assert.Equal(t, goTmp, filepath.Dir(dir))
	assert.DirExists(t, dir)
}

func TestBuildTempDir_UnusableParentFallsBackToGoDefault(t *testing.T) {
	t.Setenv("GOTMPDIR", filepath.Join(t.TempDir(), "missing"))

	assert.Empty(t, buildTempDir())
}

func TestLocalTestRunnerAdapter_RunGoTest_PassLeavesNothingInGOTMPDIR(t *testing.T) {
	goTmp := t.TempDir()
	t.Setenv("GOTMPDIR", goTmp)

	out, err := NewLocalTestRunnerAdapter().RunGoTest(context.Background(), filepath.Join("..", "..", "examples", "basic"), "./...", "")
	require.NoError(t, err, out)

	entries, err := os.ReadDir(goTmp)
	require.NoError(t, err)
	assert.Empty(t, entries)
}
