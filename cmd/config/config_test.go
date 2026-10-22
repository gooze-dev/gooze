package config

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"gooze.dev/pkg/gooze/pkg/config"
)

func execInit(t *testing.T, dir string) error {
	t.Helper()

	_, err := execInitOutput(t, dir)

	return err
}

func execInitOutput(t *testing.T, dir string) (string, error) {
	t.Helper()

	viper.Reset()
	viper.SetConfigType("yaml")
	viper.Set("output", ".gooze-reports")
	t.Cleanup(viper.Reset)

	out := &bytes.Buffer{}
	cmd := New(Deps{Dir: dir, FileName: "gooze.yaml"})
	cmd.SetOut(out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"init"})

	err := cmd.Execute()

	return out.String(), err
}

func TestConfigInit_WritesFile(t *testing.T) {
	dir := t.TempDir()

	out, err := execInitOutput(t, dir)
	require.NoError(t, err)
	require.Equal(t, "Wrote the default configuration to "+filepath.Join(dir, "gooze.yaml")+"\n", out)

	data, err := os.ReadFile(filepath.Join(dir, "gooze.yaml"))
	require.NoError(t, err)
	require.Equal(t, string(config.DefaultFile()), string(data))
}

func TestConfigInit_IgnoresEnvAndFlags(t *testing.T) {
	t.Setenv("GOOZE_RUN_PARALLEL", "8")
	t.Setenv("GOOZE_PATHS_EXCLUDE", "a b")

	dir := t.TempDir()

	viper.Reset()
	viper.SetEnvPrefix("GOOZE")
	viper.AutomaticEnv()
	viper.Set("no-cache", true)
	t.Cleanup(viper.Reset)

	cmd := New(Deps{Dir: dir, FileName: "gooze.yaml"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"init"})
	require.NoError(t, cmd.Execute())

	data, err := os.ReadFile(filepath.Join(dir, "gooze.yaml"))
	require.NoError(t, err)
	require.Equal(t, string(config.DefaultFile()), string(data))
}

func TestConfigInit_ReportsWriteFailure(t *testing.T) {
	err := execInit(t, filepath.Join(t.TempDir(), "missing-dir"))
	require.ErrorContains(t, err, "failed to write config file")
}

func TestConfigInit_ErrorsWhenFileExists(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "gooze.yaml"), []byte("existing: true\n"), 0o600))

	err := execInit(t, dir)
	require.EqualError(t, err, filepath.Join(dir, "gooze.yaml")+" already exists; edit it or remove it first")

	data, readErr := os.ReadFile(filepath.Join(dir, "gooze.yaml"))
	require.NoError(t, readErr)
	require.Equal(t, "existing: true\n", string(data), "an existing file is never overwritten")
}

func TestConfigHasInitSubcommand(t *testing.T) {
	cmd := New(Deps{Dir: ".", FileName: "gooze.yaml"})

	found := false
	for _, sub := range cmd.Commands() {
		if sub.Name() == "init" {
			found = true
		}
	}

	require.True(t, found, "config should have an init subcommand")
}
