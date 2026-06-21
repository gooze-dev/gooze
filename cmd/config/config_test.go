package config

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func execInit(t *testing.T, dir string) error {
	t.Helper()

	viper.Reset()
	viper.SetConfigType("yaml")
	viper.Set("output", ".gooze-reports")
	t.Cleanup(viper.Reset)

	cmd := New(Deps{Dir: dir, FileName: "gooze.yaml"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"init"})

	return cmd.Execute()
}

func TestConfigInit_WritesFile(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, execInit(t, dir))

	data, err := os.ReadFile(filepath.Join(dir, "gooze.yaml"))
	require.NoError(t, err)
	require.NotEmpty(t, data)
}

func TestConfigInit_ErrorsWhenFileExists(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "gooze.yaml"), []byte("existing: true\n"), 0o600))

	require.Error(t, execInit(t, dir))
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
