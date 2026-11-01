package main

import (
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
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

// goFileName is the form of a Go file's name; generated mocks keep mockery's.
var goFileName = regexp.MustCompile(`^[a-z0-9]+(_[a-z0-9]+)*\.go$`)

// unpairedTests are older test files without a source file of their name.
// Move their tests to the paired file and drop them from here; add none.
var unpairedTests = []string{
	"cmd/config_logging_test.go",
	"cmd/enum_bounds_test.go",
	"cmd/report_view_test.go",
	"cmd/shard_fuzz_test.go",
	"internal/adapter/source_fs_build_test.go",
	"internal/adapter/source_fs_diff_scope_test.go",
	"internal/adapter/source_fs_test_hash_test.go",
	"internal/adapter/test_runner_vet_test.go",
	"internal/controller/tui_integration_test.go",
	"internal/controller/ui_options_test.go",
	"internal/domain/coverage_fuzz_test.go",
	"internal/domain/diff_scope_pairing_test.go",
	"internal/domain/export_spill_test.go",
	"internal/domain/ignore_fuzz_test.go",
	"internal/domain/merge_selection_internal_test.go",
	"internal/domain/merge_threshold_test.go",
	"internal/domain/mutagen_funcs_test.go",
	"internal/domain/mutagens/comparison_minimum_test.go",
	"internal/domain/mutagens/mutants_test.go",
	"internal/domain/mutation_line_test.go",
	"internal/domain/orchestrator_deep_copy_test.go",
	"internal/domain/orchestrator_no_tests_test.go",
	"internal/domain/shard_internal_test.go",
	"internal/domain/workflow_cache_test.go",
	"internal/domain/workflow_check_tests_test.go",
	"internal/domain/workflow_coverage_test.go",
	"internal/domain/workflow_diff_test.go",
	"internal/domain/workflow_estimate_reports_test.go",
	"internal/domain/workflow_fail_fast_test.go",
	"internal/domain/workflow_interrupt_test.go",
	"internal/domain/workflow_selection_test.go",
	"internal/domain/workflow_spill_test.go",
	"internal/domain/workflow_threshold_test.go",
}

// fileLayoutProblems checks the names of the module's Go files, given by
// their slash-separated paths: snake_case names, and every test file x_test.go
// or x_internal_test.go beside its x.go. main_test.go may hold just a TestMain.
func fileLayoutProblems(files, unpaired []string) []string {
	var problems []string

	for _, file := range files {
		dir, name := path.Split(file)
		if !strings.HasPrefix(name, "mock_") && !goFileName.MatchString(name) {
			problems = append(problems, file+": name is not snake_case")
		}

		if !strings.HasSuffix(name, "_test.go") || name == "main_test.go" {
			continue
		}

		source := strings.TrimSuffix(strings.TrimSuffix(name, "_test.go"), "_internal") + ".go"
		paired := slices.Contains(files, dir+source)
		listed := slices.Contains(unpaired, file)

		switch {
		case !paired && !listed:
			problems = append(problems, file+": no "+source+" beside it; test x.go in x_test.go")
		case paired && listed:
			problems = append(problems, file+": paired now; drop it from unpairedTests")
		}
	}

	for _, file := range unpaired {
		if !slices.Contains(files, file) {
			problems = append(problems, file+": gone; drop it from unpairedTests")
		}
	}

	return problems
}

func TestFileLayoutProblems(t *testing.T) {
	for name, tt := range map[string]struct {
		files, unpaired, want []string
	}{
		"paired": {files: []string{"a/x.go", "a/x_test.go", "a/y.go", "a/y_internal_test.go", "a/main_test.go"}},
		"mock":   {files: []string{"a/mock_UI.go"}},
		"unpaired": {
			files: []string{"a/x.go", "a/x_topic_test.go", "b/x_test.go"},
			want:  []string{"a/x_topic_test.go: no x_topic.go beside it; test x.go in x_test.go", "b/x_test.go: no x.go beside it; test x.go in x_test.go"},
		},
		"listed":        {files: []string{"a/x_topic_test.go"}, unpaired: []string{"a/x_topic_test.go"}},
		"listed paired": {files: []string{"x.go", "x_test.go"}, unpaired: []string{"x_test.go"}, want: []string{"x_test.go: paired now; drop it from unpairedTests"}},
		"listed gone":   {unpaired: []string{"x_test.go"}, want: []string{"x_test.go: gone; drop it from unpairedTests"}},
		"bad names": {
			files: []string{"a/myFile.go", "a/my-file.go", "a/_x.go"},
			want:  []string{"a/myFile.go: name is not snake_case", "a/my-file.go: name is not snake_case", "a/_x.go: name is not snake_case"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := fileLayoutProblems(tt.files, tt.unpaired); !slices.Equal(got, tt.want) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFileLayout(t *testing.T) {
	var files []string

	err := filepath.WalkDir(".", func(file string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		name := d.Name()
		if d.IsDir() && file != "." && (name == "testdata" || name == "examples" || name == "vendor" || strings.HasPrefix(name, ".")) {
			return fs.SkipDir
		}

		if !d.IsDir() && strings.HasSuffix(name, ".go") {
			files = append(files, filepath.ToSlash(file))
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, problem := range fileLayoutProblems(files, unpairedTests) {
		t.Error(problem)
	}
}
