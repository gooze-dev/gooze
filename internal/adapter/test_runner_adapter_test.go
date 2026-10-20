package adapter

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These tests exercise LocalTestRunnerAdapter against the real example
// modules in the repo instead of embedding Go source in strings.

func TestLocalTestRunnerAdapter_RunGoTest_Success(t *testing.T) {
	adapter := NewLocalTestRunnerAdapter()

	// Use the basic example module and run its tests via ./...
	workDir := filepath.Join("..", "..", "examples", "basic")
	testTarget := "./..."

	out, err := adapter.RunGoTest(context.Background(), workDir, testTarget)
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

	out, err := adapter.RunGoTest(context.Background(), workDir, testTarget)
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

	out, err := adapter.RunGoTest(context.Background(), root, filepath.Join(root, "other_test.go"))
	if err != nil {
		t.Fatalf("RunGoTest() error = %v, output = %s", err, out)
	}
}
