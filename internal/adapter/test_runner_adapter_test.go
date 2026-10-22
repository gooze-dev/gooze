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
