package adapter

import (
	"os"
	"testing"
)

// gitRepoEnv are the variables git uses to find a repository. Inside a git
// hook they point at the repository running the hook, so the tests' git
// commands would act on it instead of their temporary repositories.
var gitRepoEnv = []string{
	"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR",
	"GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES",
}

// TestMain clears gitRepoEnv before the tests run, so they behave the same
// when a pre-commit hook runs them.
func TestMain(m *testing.M) {
	for _, name := range gitRepoEnv {
		_ = os.Unsetenv(name)
	}

	os.Exit(m.Run())
}

func TestMain_ClearsGitRepoEnv(t *testing.T) {
	for _, name := range gitRepoEnv {
		if value, ok := os.LookupEnv(name); ok {
			t.Errorf("%s=%s is set during the tests", name, value)
		}
	}
}
