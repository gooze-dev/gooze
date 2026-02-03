package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInitCmd_WritesConfigFile(t *testing.T) {
	tempDir := t.TempDir()
	originalWD, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tempDir))
	t.Cleanup(func() { require.NoError(t, os.Chdir(originalWD)) })

	cmd := newRootCmd()
	cmd.AddCommand(newInitCmd())
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"init"})

	err = cmd.Execute()
	require.NoError(t, err)

	targetPath := filepath.Join(tempDir, configFileName)
	t.Cleanup(func() { _ = os.Remove(targetPath) })
	info, err := os.Stat(targetPath)
	require.NoError(t, err)
	require.False(t, info.IsDir())

	contents, err := os.ReadFile(targetPath)
	require.NoError(t, err)
	require.NotEmpty(t, contents)
}

func TestInitCmd_ErrorsWhenFileExists(t *testing.T) {
	tempDir := t.TempDir()
	originalWD, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tempDir))
	t.Cleanup(func() { require.NoError(t, os.Chdir(originalWD)) })

	targetPath := filepath.Join(tempDir, configFileName)
	require.NoError(t, os.WriteFile(targetPath, []byte("existing: true\n"), 0o644))
	t.Cleanup(func() { _ = os.Remove(targetPath) })

	cmd := newRootCmd()
	cmd.AddCommand(newInitCmd())
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"init"})

	err = cmd.Execute()
	require.Error(t, err)
}
