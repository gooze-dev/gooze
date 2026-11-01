package main

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestRun_IsolatedGoCacheLeavesSharedCacheAlone runs the built gooze on a
// small module and checks that the mutants' builds add nothing to GOCACHE.
func TestRun_IsolatedGoCacheLeavesSharedCacheAlone(t *testing.T) {
	if testing.Short() {
		t.Skip("builds gooze and a module")
	}

	tmp := t.TempDir()
	gooze := filepath.Join(tmp, "gooze")
	run(t, ".", nil, "go", "build", "-o", gooze, ".")

	module := filepath.Join(tmp, "calc")
	writeFile(t, filepath.Join(module, "go.mod"), "module calc\n\ngo 1.24\n")
	writeFile(t, filepath.Join(module, "calc.go"), "package calc\n\nfunc Add(a, b int) int { return a + b }\n")
	writeFile(t, filepath.Join(module, "calc_test.go"), "package calc\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(2, 3) != 5 {\n\t\tt.Fatal(\"add\")\n\t}\n}\n")

	shared := filepath.Join(tmp, "go-build")
	env := []string{"GOCACHE=" + shared, "XDG_CACHE_HOME=" + filepath.Join(tmp, "xdg"), "GOCACHEPROG=", "GOFLAGS=-buildvcs=false"}
	run(t, module, env, "go", "test", "-count=1", "./...")

	before := cacheFiles(t, shared)

	run(t, module, append(env, "GOOZE_RUN_GO_CACHE=isolated"), gooze, "run", "--no-update-check", "-o", filepath.Join(tmp, "reports"), "./...")

	if after := cacheFiles(t, shared); !slices.Equal(before, after) {
		t.Errorf("GOCACHE changed: %d files before, %d after", len(before), len(after))
	}
}

func run(t *testing.T, dir string, env []string, name string, args ...string) {
	t.Helper()

	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(goozeFreeEnv(), env...)

	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}

func goozeFreeEnv() []string {
	return slices.DeleteFunc(os.Environ(), func(entry string) bool { return strings.HasPrefix(entry, "GOOZE_") })
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func cacheFiles(t *testing.T, dir string) []string {
	t.Helper()

	var files []string

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			files = append(files, path)
		}

		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	return files
}
