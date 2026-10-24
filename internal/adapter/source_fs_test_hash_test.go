package adapter

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	m "gooze.dev/pkg/gooze/internal/model"
)

func TestLocalSourceFSAdapter_TestHashCoversEveryTestFileOfThePackage(t *testing.T) {
	root := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte(content), 0o644))
	}

	paired := "package p\n\nimport \"testing\"\n\nfunc TestA(t *testing.T) {}\n"
	write("go.mod", "module p\n")
	write("a.go", "package p\n\nfunc A() int { return 1 }\n")
	write("a_test.go", paired)

	testHash := func() string {
		t.Helper()
		sources, err := NewLocalSourceFSAdapter().Get(context.Background(), []m.Path{m.Path(root)})
		require.NoError(t, err)
		require.Len(t, sources, 1)
		require.NotNil(t, sources[0].Test)
		assert.Equal(t, filepath.Join(root, "a_test.go"), sources[0].Test.FullPath, "the paired test stays the test file")

		return sources[0].Test.Hash
	}

	alone := testHash()
	assert.Equal(t, hashBytes([]byte(paired)), alone, "with only the paired test, the hash is unchanged")

	write("a_linux_test.go", "package p\n\nimport \"testing\"\n\nfunc TestLinux(t *testing.T) {}\n")
	added := testHash()
	assert.NotEqual(t, alone, added, "adding a test file changes the hash")

	write("a_linux_test.go", "package p\n\nimport \"testing\"\n\nfunc TestLinux(t *testing.T) { t.Log() }\n")
	assert.NotEqual(t, added, testHash(), "editing another test file changes the hash")

	require.NoError(t, os.Remove(filepath.Join(root, "a_linux_test.go")))
	assert.Equal(t, alone, testHash(), "deleting it brings the hash back")

	write("b_test.go", "package p\n")
	write("c_test.go", "package p\n")
	first := testHash()
	require.NoError(t, os.Rename(filepath.Join(root, "c_test.go"), filepath.Join(root, "d_test.go")))
	assert.NotEqual(t, first, testHash(), "renaming a test file changes the hash")
}
