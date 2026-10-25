package gocache

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testAction = sha256.Sum256([]byte("action"))
	testOutput = sha256.Sum256([]byte("output"))
	testTime   = time.Unix(1700000000, 123)
)

// writeEntry writes an entry the way Go's disk cache does.
func writeEntry(t *testing.T, dir string, action, output []byte, body string) {
	t.Helper()

	index := fmt.Sprintf("v1 %x %x %20d %20d\n", action, output, len(body), testTime.UnixNano())
	writeTestFile(t, cacheFile(dir, action, "a"), index)
	writeTestFile(t, cacheFile(dir, output, "d"), body)
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func TestCacheFile(t *testing.T) {
	id := []byte{0xab, 0x01}
	assert.Equal(t, filepath.Join("dir", "ab", "ab01-d"), cacheFile("dir", id, "d"))
}

func TestReadEntry_Hit(t *testing.T) {
	dir := t.TempDir()
	writeEntry(t, dir, testAction[:], testOutput[:], "body")

	got, ok := readEntry(dir, testAction[:])
	require.True(t, ok)
	assert.Equal(t, entry{outputID: testOutput[:], size: 4, time: testTime, path: cacheFile(dir, testOutput[:], "d")}, got)
}

func TestReadEntry_Misses(t *testing.T) {
	index := func(version string, action, output []byte, size int, time string) string {
		return fmt.Sprintf("%s %x %x %20d %20s\n", version, action, output, size, time)
	}
	nanos := fmt.Sprint(testTime.UnixNano())
	other := sha256.Sum256([]byte("other"))

	tests := map[string]string{
		"empty":           "",
		"version":         index("v2", testAction[:], testOutput[:], 4, nanos),
		"other action":    index("v1", other[:], testOutput[:], 4, nanos),
		"bad output":      fmt.Sprintf("v1 %x zz %20d %20s\n", testAction[:], 4, nanos),
		"short output":    index("v1", testAction[:], testOutput[:1], 4, nanos),
		"bad size":        fmt.Sprintf("v1 %x %x %20s %20s\n", testAction[:], testOutput[:], "x", nanos),
		"bad time":        index("v1", testAction[:], testOutput[:], 4, "x"),
		"size mismatch":   index("v1", testAction[:], testOutput[:], 5, nanos),
		"extra field":     index("v1", testAction[:], testOutput[:], 4, nanos+" 1"),
		"missing a field": fmt.Sprintf("v1 %x %x %20d\n", testAction[:], testOutput[:], 4),
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeEntry(t, dir, testAction[:], testOutput[:], "body")
			writeTestFile(t, cacheFile(dir, testAction[:], "a"), content)

			_, ok := readEntry(dir, testAction[:])
			assert.False(t, ok)
		})
	}
}

func TestReadEntry_MissingFiles(t *testing.T) {
	dir := t.TempDir()

	_, ok := readEntry(dir, testAction[:])
	assert.False(t, ok, "no action file")

	writeEntry(t, dir, testAction[:], testOutput[:], "body")
	require.NoError(t, os.Remove(cacheFile(dir, testOutput[:], "d")))

	_, ok = readEntry(dir, testAction[:])
	assert.False(t, ok, "no output file")
}

// A directory output is a cached executable, which the go command reads only
// from its own disk cache.
func TestReadEntry_DirectoryOutput(t *testing.T) {
	dir := t.TempDir()
	writeEntry(t, dir, testAction[:], testOutput[:], "")
	output := cacheFile(dir, testOutput[:], "d")
	require.NoError(t, os.Remove(output))
	require.NoError(t, os.Mkdir(output, 0o755))

	_, ok := readEntry(dir, testAction[:])
	assert.False(t, ok)
}

func TestReadEntry_LeavesFilesAlone(t *testing.T) {
	dir := t.TempDir()
	writeEntry(t, dir, testAction[:], testOutput[:], "body")
	old := time.Now().Add(-48 * time.Hour)
	require.NoError(t, os.Chtimes(cacheFile(dir, testOutput[:], "d"), old, old))

	_, ok := readEntry(dir, testAction[:])
	require.True(t, ok)

	info, err := os.Stat(cacheFile(dir, testOutput[:], "d"))
	require.NoError(t, err)
	assert.True(t, info.ModTime().Equal(old))
}
