package adapter

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gooze.dev/pkg/gooze/internal/domain"
)

// These tests exercise LocalTestRunnerAdapter against the real example
// modules in the repo instead of embedding Go source in strings.

func TestLocalTestRunnerAdapter_RunGoTest_Success(t *testing.T) {
	adapter := NewLocalTestRunnerAdapter()

	// Use the basic example module and run its tests via ./...
	workDir := filepath.Join("..", "..", "examples", "basic")
	testTarget := "./..."

	out, err := adapter.RunGoTest(context.Background(), workDir, testTarget, "")
	if err != nil {
		t.Fatalf("RunGoTest() error = %v, output = %s", err, out)
	}

	// We don't assert exact text, just that it looks like go test output.
	if !strings.Contains(out, "=== RUN") && !strings.Contains(out, "ok ") {
		t.Fatalf("RunGoTest() output does not look like go test output: %q", out)
	}
}

func TestLocalTestRunnerAdapter_RunGoTest_Failure(t *testing.T) {
	adapter := NewLocalTestRunnerAdapter()

	// Same example module, but point go test at a non-existent package.
	workDir := filepath.Join("..", "..", "examples", "basic")
	testTarget := filepath.Join(workDir, "does_not_exist", "missing_test.go")

	out, err := adapter.RunGoTest(context.Background(), workDir, testTarget, "")
	if err == nil {
		t.Fatalf("RunGoTest() expected error for missing test target, got nil (output=%s)", out)
	}

	if out == "" {
		t.Fatalf("RunGoTest() expected some diagnostic output for failure, got empty string")
	}
}

func TestLocalTestRunnerAdapter_RunGoTest_RunsWholePackage(t *testing.T) {
	root := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	write("go.mod", "module calc\n\ngo 1.26\n")
	write("calc.go", "package calc\n\nfunc Add(a, b int) int { return a + b }\n")
	write("other_test.go", "package calc\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1, 2) != 3 {\n\t\tt.Fatal(\"add\")\n\t}\n}\n")

	adapter := NewLocalTestRunnerAdapter()

	out, err := adapter.RunGoTest(context.Background(), root, filepath.Join(root, "other_test.go"), "")
	if err != nil {
		t.Fatalf("RunGoTest() error = %v, output = %s", err, out)
	}
}

func TestLocalTestRunnerAdapter_RunGoTest_Overlay(t *testing.T) {
	root := t.TempDir()
	write := func(path, content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	source := filepath.Join(root, "calc.go")
	original := "package calc\n\nfunc Add(a, b int) int { return a + b }\n"

	write(filepath.Join(root, "go.mod"), "module calc\n\ngo 1.26\n")
	write(source, original)
	write(filepath.Join(root, "calc_test.go"), "package calc\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1, 2) != 3 {\n\t\tt.Fatal(\"add\")\n\t}\n}\n")

	mutantDir := t.TempDir()
	mutant := filepath.Join(mutantDir, "mutant.go")
	overlay := filepath.Join(mutantDir, "overlay.json")
	write(mutant, "package calc\n\nfunc Add(a, b int) int { return a - b }\n")
	write(overlay, `{"Replace":{"`+filepath.ToSlash(source)+`":"`+filepath.ToSlash(mutant)+`"}}`)

	adapter := NewLocalTestRunnerAdapter()

	out, err := adapter.RunGoTest(context.Background(), root, source, overlay)
	if err == nil {
		t.Fatalf("RunGoTest() with the mutant overlay passed, want the test to fail (output=%s)", out)
	}

	if !strings.Contains(out, "--- FAIL: TestAdd") {
		t.Fatalf("RunGoTest() output = %q, want TestAdd to fail", out)
	}

	content, readErr := os.ReadFile(source)
	if readErr != nil {
		t.Fatal(readErr)
	}

	if string(content) != original {
		t.Fatalf("source on disk changed to %q", content)
	}
}

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

func TestPackageArg(t *testing.T) {
	sep := string(filepath.Separator)

	for name, tt := range map[string]struct{ workDir, testFile, want string }{
		"inside the work dir": {"/w", "/w/p/x_test.go", "." + sep + "p"},
		"the work dir":        {"/w", "/w/x_test.go", "." + sep + "."},
		"not relative to it":  {"w", "/abs/x_test.go", "/abs"},
	} {
		assert.Equal(t, tt.want, packageArg(tt.workDir, tt.testFile), name)
	}
}

func TestNewGoTestCommand(t *testing.T) {
	cmd := newGoTestCommand(context.Background(), "/w", "/w/p/x_test.go", "", nil)

	assert.Equal(t, "/w", cmd.Dir)
	assert.Equal(t, 5*time.Second, cmd.WaitDelay)
	assert.Equal(t, []string{"test", "-count=1", "-vet=off", "-v", "." + string(filepath.Separator) + "p"}, cmd.Args[1:])
}

func TestNewGoTestCommand_TestFlags(t *testing.T) {
	cmd := newGoTestCommand(context.Background(), "/w", "/w/p/x_test.go", "o.json", []string{"-short", "-tags=integration"})

	assert.Equal(t, []string{"test", "-count=1", "-vet=off", "-v", "-overlay", "o.json", "-short", "-tags=integration", "." + string(filepath.Separator) + "p"}, cmd.Args[1:],
		"after gooze's own flags, before the package")
}

func TestLocalTestRunnerAdapter_RunGoTest_UsesTheTestFlags(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/p\n\ngo 1.24\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "p_test.go"), []byte(`package p

import "testing"

func TestLong(t *testing.T) {
	if !testing.Short() {
		t.Fatal("runs only with -short")
	}
}
`), 0o600))

	runner := NewLocalTestRunnerAdapter()

	_, err := runner.RunGoTest(context.Background(), dir, filepath.Join(dir, "p_test.go"), "")
	require.Error(t, err, "without -short")

	runner.SetTestFlags([]string{"-short"})

	out, err := runner.RunGoTest(context.Background(), dir, filepath.Join(dir, "p_test.go"), "")
	require.NoError(t, err, out)
}

// memoryHungryModule writes a module whose test takes 128 MiB every 50 ms
// until it holds 1.5 GiB, then passes.
func memoryHungryModule(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/hungry\n\ngo 1.24\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "hungry_test.go"), []byte(`package hungry

import (
	"testing"
	"time"
)

func TestHungry(t *testing.T) {
	var held [][]byte

	for range 12 {
		buf := make([]byte, 128<<20)
		for i := range buf {
			buf[i] = 1
		}

		held = append(held, buf)

		time.Sleep(50 * time.Millisecond)
	}

	t.Log("held", len(held))
}
`), 0o600))

	return dir
}

func TestLocalTestRunnerAdapter_RunGoTest_MemoryLimit(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the memory limit is enforced on linux only")
	}

	dir := memoryHungryModule(t)
	runner := NewLocalTestRunnerAdapter()
	runner.SetMemoryLimit(512 << 20)

	out, err := runner.RunGoTest(context.Background(), dir, filepath.Join(dir, "hungry_test.go"), "")

	require.ErrorIs(t, err, domain.ErrMemoryLimit, out)
	require.NotContains(t, out, "--- PASS", "the test was killed before it finished")
}

func TestLocalTestRunnerAdapter_RunGoTest_UnderMemoryLimit(t *testing.T) {
	dir := memoryHungryModule(t)
	runner := NewLocalTestRunnerAdapter()

	out, err := runner.RunGoTest(context.Background(), dir, filepath.Join(dir, "hungry_test.go"), "")
	require.NoError(t, err, "no limit: %s", out)

	runner.SetMemoryLimit(8 << 30)

	out, err = runner.RunGoTest(context.Background(), dir, filepath.Join(dir, "hungry_test.go"), "")
	require.NoError(t, err, "under the limit: %s", out)
	require.Contains(t, out, "--- PASS")
}
