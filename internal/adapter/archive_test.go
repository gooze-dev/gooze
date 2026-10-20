package adapter

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTarGzRoundTrip(t *testing.T) {
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "a.yaml"), []byte("alpha"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(src, "sub"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(src, "sub", "b.yaml"), []byte("bravo"), 0o600))

	archive := filepath.Join(t.TempDir(), "reports.tgz")
	require.NoError(t, tarGz(src, archive))

	dst := t.TempDir()
	require.NoError(t, unTarGz(archive, dst))

	a, err := os.ReadFile(filepath.Join(dst, "a.yaml"))
	require.NoError(t, err)
	require.Equal(t, "alpha", string(a))

	b, err := os.ReadFile(filepath.Join(dst, "sub", "b.yaml"))
	require.NoError(t, err)
	require.Equal(t, "bravo", string(b))
}

func TestArchiveEntryPathRejectsTraversal(t *testing.T) {
	for _, bad := range []string{"../escape.txt", "..", "sub/../../x", "/abs.txt", "", "."} {
		_, err := archiveEntryPath(bad)
		require.Error(t, err, bad)
	}

	ok, err := archiveEntryPath("sub/./file.txt")
	require.NoError(t, err)
	require.Equal(t, filepath.Join("sub", "file.txt"), ok)
}

func TestUnTarGzIntoRelativeDotDir(t *testing.T) {
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "a.yaml"), []byte("alpha"), 0o600))

	archive := filepath.Join(t.TempDir(), "reports.tgz")
	require.NoError(t, tarGz(src, archive))

	dst := t.TempDir()
	t.Chdir(dst)

	require.NoError(t, unTarGz(archive, "."))

	a, err := os.ReadFile(filepath.Join(dst, "a.yaml"))
	require.NoError(t, err)
	require.Equal(t, "alpha", string(a))
}

func TestTarGzSkipsSymlinks(t *testing.T) {
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "a.yaml"), []byte("alpha"), 0o600))
	require.NoError(t, os.Symlink(filepath.Join(src, "a.yaml"), filepath.Join(src, "link.yaml")))
	require.NoError(t, os.Symlink(src, filepath.Join(src, "dirlink")))

	archive := filepath.Join(t.TempDir(), "reports.tgz")
	require.NoError(t, tarGz(src, archive))

	dst := t.TempDir()
	require.NoError(t, unTarGz(archive, dst))

	entries, err := os.ReadDir(dst)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "a.yaml", entries[0].Name())
}

func TestUnTarGzPreservesModesAndOverwritesReadOnly(t *testing.T) {
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "exec.sh"), []byte("#!/bin/sh"), 0o755)) //nolint:gosec // test fixture
	require.NoError(t, os.WriteFile(filepath.Join(src, "ro.yaml"), []byte("v1"), 0o444))

	archive := filepath.Join(t.TempDir(), "reports.tgz")
	require.NoError(t, tarGz(src, archive))

	dst := t.TempDir()
	require.NoError(t, unTarGz(archive, dst))

	info, err := os.Stat(filepath.Join(dst, "exec.sh"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o755), info.Mode().Perm())

	info, err = os.Stat(filepath.Join(dst, "ro.yaml"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o644), info.Mode().Perm())

	// Pulling a second time over the existing tree must succeed.
	require.NoError(t, unTarGz(archive, dst))
}

func TestUnTarGzDoesNotFollowSymlinkedDir(t *testing.T) {
	src := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(src, "sub"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(src, "sub", "b.yaml"), []byte("bravo"), 0o600))

	archive := filepath.Join(t.TempDir(), "reports.tgz")
	require.NoError(t, tarGz(src, archive))

	outside := t.TempDir()
	dst := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(dst, "sub")))

	require.Error(t, unTarGz(archive, dst))
	_, err := os.Stat(filepath.Join(outside, "b.yaml"))
	require.True(t, os.IsNotExist(err), "extraction escaped through a symlinked dir")
}
