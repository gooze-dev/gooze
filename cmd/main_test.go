package cmd

import (
	"os"
	"strings"
	"testing"
)

// TestMain clears every GOOZE_* variable before the tests run: viper reads
// them, so a value from the developer's shell (GOOZE_NO_UPDATE_CHECK=true is
// common) would replace the defaults the tests expect. Tests that need one
// set it with t.Setenv.
func TestMain(m *testing.M) {
	for _, entry := range os.Environ() {
		if name, _, _ := strings.Cut(entry, "="); strings.HasPrefix(name, envPrefix+"_") {
			_ = os.Unsetenv(name)
		}
	}

	os.Exit(m.Run())
}

func TestMain_ClearsGoozeEnv(t *testing.T) {
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, envPrefix+"_") {
			t.Errorf("%s is set during the tests", entry)
		}
	}
}
