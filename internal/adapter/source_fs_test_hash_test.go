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

func TestLocalSourceFSAdapter_TestHashCoversTheOtherSourcesOfThePackage(t *testing.T) {
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

		for _, source := range sources {
			if filepath.Base(source.Origin.FullPath) == "a.go" {
				require.NotNil(t, source.Test)

				return source.Test.Hash
			}
		}

		require.Fail(t, "a.go is not among the sources")

		return ""
	}

	alone := testHash()
	assert.Equal(t, hashBytes([]byte(paired)), alone, "the source itself is not part of its test hash")

	write("a.go", "package p\n\nfunc A() int { return 2 }\n")
	assert.Equal(t, alone, testHash(), "editing the source itself leaves its test hash alone")

	write("b.go", "package p\n\nfunc clamp(n int) int { return 0 }\n")
	added := testHash()
	assert.NotEqual(t, alone, added, "adding a source to the package changes the hash")

	write("b.go", "package p\n\nfunc clamp(n int) int { return n }\n")
	edited := testHash()
	assert.NotEqual(t, added, edited, "editing another source of the package changes the hash")

	require.NoError(t, os.Rename(filepath.Join(root, "b.go"), filepath.Join(root, "c.go")))
	assert.NotEqual(t, edited, testHash(), "renaming another source changes the hash")

	require.NoError(t, os.Remove(filepath.Join(root, "c.go")))
	assert.Equal(t, alone, testHash(), "deleting it brings the hash back")

	write("notes.txt", "not go")
	assert.Equal(t, alone, testHash(), "a file that is not Go changes nothing")

	write("0.go", "package p\n")
	assert.NotEqual(t, alone, testHash(), "a source that sorts before the file itself counts too")
}
