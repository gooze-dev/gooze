package adapter

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalTestRunnerAdapter_RunGoTest_VetDoesNotHideTheTests(t *testing.T) {
	root := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	// A logical mutant of s == "a" || s == "b": vet calls it a suspect and,
	// and go test would stop at vet before running the tests that catch it.
	write("go.mod", "module pick\n\ngo 1.26\n")
	write("pick.go", "package pick\n\nfunc Pick(s string) bool { return s == \"a\" && s == \"b\" }\n")
	write("pick_test.go", "package pick\n\nimport \"testing\"\n\nfunc TestPick(t *testing.T) {\n\tif !Pick(\"a\") {\n\t\tt.Fatal(\"a\")\n\t}\n}\n")

	out, err := NewLocalTestRunnerAdapter().RunGoTest(context.Background(), root, filepath.Join(root, "pick_test.go"), "")
	if err == nil {
		t.Fatalf("RunGoTest() expected the test to fail, output = %s", out)
	}

	if !strings.Contains(out, "--- FAIL: TestPick") || strings.Contains(out, "[build failed]") {
		t.Fatalf("RunGoTest() should run the tests, not stop at vet; output = %s", out)
	}
}
