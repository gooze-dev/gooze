package config

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gooze.dev/pkg/gooze/pkg/config"
)

// showLines runs config show with the schema's defaults, yaml as gooze.yaml
// and args, and returns its lines by key.
func showLines(t *testing.T, yaml string, args ...string) map[string]string {
	t.Helper()

	viper.Reset()
	t.Cleanup(viper.Reset)

	file := filepath.Join(t.TempDir(), "gooze.yaml")
	require.NoError(t, os.WriteFile(file, []byte(yaml), 0o600))

	viper.SetConfigFile(file)
	viper.AutomaticEnv()
	viper.SetEnvPrefix("GOOZE")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))

	for _, key := range config.Keys() {
		field, _ := config.Lookup(key)
		viper.SetDefault(key, field.Default)
	}

	require.NoError(t, viper.ReadInConfig())

	out := &bytes.Buffer{}
	cmd := New(Deps{EnvPrefix: "GOOZE", FlagKeys: map[string]string{"output": config.KeyOutput}})
	cmd.PersistentFlags().String("output", "", "")
	require.NoError(t, viper.BindPFlag(config.KeyOutput, cmd.PersistentFlags().Lookup("output")))
	cmd.SetOut(out)
	cmd.SetArgs(append([]string{"show"}, args...))
	require.NoError(t, cmd.Execute())

	lines := map[string]string{}

	for line := range strings.SplitSeq(strings.TrimSpace(out.String()), "\n") {
		lines[strings.Fields(line)[0]] = strings.Join(strings.Fields(line), " ")
	}

	return lines
}

func TestConfigShow_ListsEveryKey(t *testing.T) {
	lines := showLines(t, "")

	assert.Equal(t, "KEY VALUE SOURCE ENV", lines["KEY"])
	assert.Len(t, lines, len(config.Keys())+1)

	for _, key := range config.Keys() {
		assert.Contains(t, lines, key)
	}
}

func TestConfigShow_SaysWhereEachValueComesFrom(t *testing.T) {
	t.Setenv("GOOZE_RUN_PARALLEL", "7")
	t.Setenv("GOOZE_RUN_DIFF", "")

	lines := showLines(t, "run:\n  parallel: 3\n  mutation_timeout: 9\nno-cache: true\n", "--output", "out")

	assert.Equal(t, "output out flag GOOZE_OUTPUT", lines[config.KeyOutput])
	assert.Equal(t, "run.parallel 7 env GOOZE_RUN_PARALLEL", lines[config.KeyRunParallel], "env wins over the file")
	assert.Equal(t, "run.mutation_timeout 9 gooze.yaml GOOZE_RUN_MUTATION_TIMEOUT", lines[config.KeyRunMutationTimeout])
	assert.Equal(t, "no-cache true gooze.yaml GOOZE_NO_CACHE", lines[config.KeyNoCache], "a kebab-case key")
	assert.Equal(t, "run.workspace overlay default GOOZE_RUN_WORKSPACE", lines[config.KeyRunWorkspace])
	assert.Contains(t, lines[config.KeyRunDiff], " default ", "an empty variable doesn't count")
}

func TestConfigShow_AFlagWinsOverTheEnvironment(t *testing.T) {
	t.Setenv("GOOZE_OUTPUT", "fromenv")

	assert.Equal(t, "output out flag GOOZE_OUTPUT", showLines(t, "", "--output", "out")[config.KeyOutput])
	assert.Equal(t, "output fromenv env GOOZE_OUTPUT", showLines(t, "")[config.KeyOutput])
}

func TestConfigShow_ShowsAnEmptyValue(t *testing.T) {
	assert.Equal(t, `run.diff "" default GOOZE_RUN_DIFF`, showLines(t, "")[config.KeyRunDiff])
}
