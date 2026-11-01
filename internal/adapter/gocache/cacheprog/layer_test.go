package cacheprog

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLayer_PutThenGet(t *testing.T) {
	l := Layer{Dir: filepath.Join(t.TempDir(), "layer")}

	path, err := l.put(testAction[:], testOutput[:], []byte("body"), testTime)
	require.NoError(t, err)
	assert.Equal(t, cacheFile(l.Dir, testOutput[:], "d"), path)

	got, ok := l.get(testAction[:])
	require.True(t, ok)
	assert.Equal(t, entry{outputID: testOutput[:], size: 4, time: testTime, path: path}, got)

	body, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "body", string(body))

	files, _, err := l.files()
	require.NoError(t, err)
	assert.Len(t, files, 2, "no temporary files left")
}

func TestLayer_PutReplaces(t *testing.T) {
	l := Layer{Dir: t.TempDir()}

	_, err := l.put(testAction[:], testOutput[:], []byte("old body"), testTime)
	require.NoError(t, err)
	_, err = l.put(testAction[:], testOutput[:], []byte("body"), testTime)
	require.NoError(t, err)

	got, ok := l.get(testAction[:])
	require.True(t, ok)
	assert.Equal(t, int64(4), got.size)
}

func TestLayer_PutFails(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	writeTestFile(t, file, "")

	_, err := Layer{Dir: file}.put(testAction[:], testOutput[:], []byte("body"), testTime)
	require.Error(t, err)

	dir := t.TempDir()
	writeTestFile(t, cacheFile(dir, testOutput[:], "d")+"/x", "")

	_, err = Layer{Dir: dir}.put(testAction[:], testOutput[:], []byte("body"), testTime)
	require.Error(t, err, "the output path is a non-empty directory")
}

func TestLayer_GetMarksUse(t *testing.T) {
	l := Layer{Dir: t.TempDir()}
	_, err := l.put(testAction[:], testOutput[:], []byte("body"), testTime)
	require.NoError(t, err)

	old := time.Now().Add(-48 * time.Hour)
	for _, kind := range []string{"a", "d"} {
		id := map[string][]byte{"a": testAction[:], "d": testOutput[:]}[kind]
		require.NoError(t, os.Chtimes(cacheFile(l.Dir, id, kind), old, old))
	}

	_, ok := l.get(testAction[:])
	require.True(t, ok)

	for _, path := range []string{cacheFile(l.Dir, testAction[:], "a"), cacheFile(l.Dir, testOutput[:], "d")} {
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.WithinDuration(t, time.Now(), info.ModTime(), time.Minute, path)
	}
}

func TestLayer_GetMiss(t *testing.T) {
	_, ok := Layer{Dir: t.TempDir()}.get(testAction[:])
	assert.False(t, ok)
}

// layerWith returns a layer holding a file of size bytes per name, each last
// used age ago.
func layerWith(t *testing.T, now time.Time, files map[string]struct {
	size int
	age  time.Duration
}) Layer {
	t.Helper()

	dir := t.TempDir()
	for name, f := range files {
		path := filepath.Join(dir, name[:2], name)
		writeTestFile(t, path, string(make([]byte, f.size)))
		require.NoError(t, os.Chtimes(path, now.Add(-f.age), now.Add(-f.age)))
	}

	return Layer{Dir: dir}
}

func names(t *testing.T, l Layer) []string {
	t.Helper()

	files, _, err := l.files()
	require.NoError(t, err)

	var got []string
	for _, f := range files {
		got = append(got, filepath.Base(f.path))
	}

	return got
}

func TestLayer_Trim(t *testing.T) {
	now := time.Now()
	type file = struct {
		size int
		age  time.Duration
	}

	tests := []struct {
		name  string
		limit int64
		files map[string]file
		want  []string
	}{
		{
			name:  "under the limit",
			limit: 30,
			files: map[string]file{"aa-a": {10, 3 * time.Hour}, "bb-d": {20, 2 * time.Hour}},
			want:  []string{"aa-a", "bb-d"},
		},
		{
			name:  "least recently used first",
			limit: 25,
			files: map[string]file{"aa-a": {10, 2 * time.Hour}, "bb-d": {10, 3 * time.Hour}, "cc-d": {10, 4 * time.Hour}},
			want:  []string{"aa-a", "bb-d"},
		},
		{
			name:  "down to the limit",
			limit: 20,
			files: map[string]file{"aa-a": {10, 2 * time.Hour}, "bb-d": {10, 3 * time.Hour}, "cc-d": {10, 4 * time.Hour}},
			want:  []string{"aa-a", "bb-d"},
		},
		{
			name:  "spares the last hour",
			limit: 0,
			files: map[string]file{"aa-a": {10, time.Hour - time.Minute}, "bb-d": {10, time.Hour}},
			want:  []string{"aa-a"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := layerWith(t, now, tt.files)

			require.NoError(t, l.Trim(tt.limit, now))
			assert.ElementsMatch(t, tt.want, names(t, l))
		})
	}
}

func TestLayer_TrimMissingDir(t *testing.T) {
	require.NoError(t, Layer{Dir: filepath.Join(t.TempDir(), "none")}.Trim(0, time.Now()))
}

func TestLayer_FilesTotal(t *testing.T) {
	now := time.Now()
	l := layerWith(t, now, map[string]struct {
		size int
		age  time.Duration
	}{"aa-a": {10, 2 * time.Hour}, "bb-d": {5, time.Hour}})

	files, total, err := l.files()
	require.NoError(t, err)
	assert.Equal(t, int64(15), total)
	require.Len(t, files, 2)
	assert.Equal(t, "aa-a", filepath.Base(files[0].path), "oldest first")
}

func TestLayer_FilesError(t *testing.T) {
	dir := t.TempDir()
	locked := filepath.Join(dir, "aa")
	require.NoError(t, os.Mkdir(locked, 0o000))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	if _, err := os.ReadDir(locked); err == nil {
		t.Skip("running as root")
	}

	_, _, err := Layer{Dir: dir}.files()
	require.Error(t, err)
	require.Error(t, Layer{Dir: dir}.Trim(0, time.Now()))
}

func TestLayer_TrimReportsARemoveFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root removes files from read-only directories")
	}

	dir := t.TempDir()
	sub := filepath.Join(dir, "aa")
	path := filepath.Join(sub, "aa-d")
	writeTestFile(t, path, "body")

	old := time.Now().Add(-24 * time.Hour)
	require.NoError(t, os.Chtimes(path, old, old))
	require.NoError(t, os.Chmod(sub, 0o500)) //nolint:gosec // the test needs a read-only directory.

	t.Cleanup(func() { _ = os.Chmod(sub, 0o700) }) //nolint:gosec // lets t.TempDir clean up.

	require.Error(t, (&Layer{Dir: dir}).Trim(0, time.Now()))
}

func TestLayer_TrimSubtractsEachRemovedFile(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()

	for i, name := range []string{"aa-d", "bb-d", "cc-d"} {
		path := filepath.Join(dir, name)
		writeTestFile(t, path, "0123456789")

		used := now.Add(-time.Duration(3-i) * 24 * time.Hour)
		require.NoError(t, os.Chtimes(path, used, used))
	}

	require.NoError(t, Layer{Dir: dir}.Trim(15, now))

	assert.NoFileExists(t, filepath.Join(dir, "aa-d"))
	assert.NoFileExists(t, filepath.Join(dir, "bb-d"))
	assert.FileExists(t, filepath.Join(dir, "cc-d"))
}
