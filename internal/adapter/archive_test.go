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

func TestSafeJoinRejectsTraversal(t *testing.T) {
	_, err := safeJoin("/tmp/dest", "../escape.txt")
	require.Error(t, err)

	ok, err := safeJoin("/tmp/dest", "sub/file.txt")
	require.NoError(t, err)
	require.Equal(t, filepath.Join("/tmp/dest", "sub", "file.txt"), ok)
}
