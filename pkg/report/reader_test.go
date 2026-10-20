package report

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

var fixtureDir = filepath.Join("..", "..", "testdata", "report")

func TestNewDirReader_ReadsFixture(t *testing.T) {
	var reader Reader = NewDirReader()

	reports, err := reader.ReadDir(fixtureDir)
	require.NoError(t, err)
	require.Len(t, reports, 2)

	var keys []string

	require.NoError(t, reader.WalkDir(fixtureDir, func(_ string, r Report) error {
		keys = append(keys, r.Source.Key())
		return nil
	}))
	assert.Equal(t, []string{"calc/sub.go", "calc/add.go"}, keys)

	index, err := reader.ReadIndex(fixtureDir)
	require.NoError(t, err)
	assert.Equal(t, 6, index.TotalMutations)
}

func TestMockReader_FakesReader(t *testing.T) {
	mock := NewMockReader(t)
	mock.EXPECT().ReadIndex("reports").Return(Index{TotalMutations: 3}, nil)
	mock.EXPECT().ReadDir("broken").Return(nil, errors.New("boom"))

	var reader Reader = mock

	index, err := reader.ReadIndex("reports")
	require.NoError(t, err)
	assert.Equal(t, 3, index.TotalMutations)

	_, err = reader.ReadDir("broken")
	require.EqualError(t, err, "boom")
}

func TestReportFormat_RoundTripsFixtureByteForByte(t *testing.T) {
	visited := 0

	err := WalkDir(fixtureDir, func(path string, r Report) error {
		visited++

		want, err := os.ReadFile(path)
		require.NoError(t, err)

		got, err := yaml.Marshal(r)
		require.NoError(t, err)
		assert.Equal(t, string(want), string(got), path)

		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 2, visited)
}

func TestIndexFormat_RoundTripsFixtureByteForByte(t *testing.T) {
	index, err := ReadIndex(fixtureDir)
	require.NoError(t, err)

	want, err := os.ReadFile(filepath.Join(fixtureDir, IndexFileName))
	require.NoError(t, err)

	got, err := yaml.Marshal(index)
	require.NoError(t, err)
	assert.Equal(t, string(want), string(got))
}

func TestReadIndex_Fields(t *testing.T) {
	index, err := ReadIndex(fixtureDir)
	require.NoError(t, err)

	assert.Equal(t, 6, index.TotalMutations)
	assert.Equal(t, 1, index.KilledMutations)
	assert.Equal(t, 1, index.SurvivedMutations)
	assert.Equal(t, 2, index.FailedMutations)
	assert.Equal(t, 1, index.IgnoredMutations)
	assert.Equal(t, 1, index.NotCoveredMutations)
	require.Len(t, index.Result, 2)
	assert.Equal(t, "aaaa", index.Result[0].SourceHex)
	assert.Equal(t, "arithmetic", index.Result[0].Mutations[0].MutationName)
	assert.Equal(t, []string{"c8efb49977c64ef4.yaml"}, index.Result[0].Mutations[0].MutationReports)
}

func TestReadIndex_Errors(t *testing.T) {
	_, err := ReadIndex(t.TempDir())
	require.ErrorIs(t, err, os.ErrNotExist)

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, IndexFileName), []byte("total_mutations: [\n"), 0o600))

	_, err = ReadIndex(dir)
	require.ErrorContains(t, err, "unmarshal index file")
}

func TestReadFile_DecodesFields(t *testing.T) {
	r, err := ReadFile(filepath.Join(fixtureDir, "c8efb49977c64ef4.yaml"))
	require.NoError(t, err)

	require.NotNil(t, r.Source.Origin)
	assert.Equal(t, File{ShortPath: "calc/add.go", FullPath: "/repo/calc/add.go", Hash: "aaaa"}, *r.Source.Origin)
	require.NotNil(t, r.Source.Test)
	assert.Equal(t, "calc/add_test.go", r.Source.Test.ShortPath)
	require.NotNil(t, r.Source.Package)
	assert.Equal(t, "calc", *r.Source.Package)
	require.NotNil(t, r.Diff)
	assert.Equal(t, "--- original\n+++ mutated\n@@ -1 +1 @@\n-a + b\n+a - b\n", string(*r.Diff))
	assert.Equal(t, []Result{
		{Name: "arithmetic", Version: 1, Mutations: []MutationResult{{MutationID: "m1", Status: Killed}, {MutationID: "m2", Status: Survived}}},
		{Name: "boolean", Version: 1, Mutations: []MutationResult{{MutationID: "m3", Status: Error}}},
	}, r.Result)
}

func TestReadFile_Errors(t *testing.T) {
	_, err := ReadFile(filepath.Join(t.TempDir(), "missing.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)
	require.ErrorContains(t, err, "read report file")

	path := filepath.Join(t.TempDir(), "bad.yaml")
	require.NoError(t, os.WriteFile(path, []byte("source: [\n"), 0o600))

	_, err = ReadFile(path)
	require.ErrorContains(t, err, "unmarshal report file "+path)
}

func TestWalkDir_SelectsReportFilesInNameOrder(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
	}

	write("b.yaml", "result:\n    - name: b\n")
	write("a.yml", "result:\n    - name: a\n")
	write(IndexFileName, "total_mutations: 1\n")
	write("notes.txt", "not a report")
	require.NoError(t, os.Mkdir(filepath.Join(dir, "sub.yaml"), 0o750))

	var names []string

	require.NoError(t, WalkDir(dir, func(path string, r Report) error {
		assert.Equal(t, dir, filepath.Dir(path))
		names = append(names, r.Result[0].Name)

		return nil
	}))
	assert.Equal(t, []string{"a", "b"}, names)
}

func TestWalkDir_Errors(t *testing.T) {
	err := WalkDir(filepath.Join(t.TempDir(), "missing"), func(string, Report) error { return nil })
	require.ErrorIs(t, err, os.ErrNotExist)
	require.ErrorContains(t, err, "read reports directory")

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte("source: [\n"), 0o600))
	require.ErrorContains(t, WalkDir(dir, func(string, Report) error { return nil }), "unmarshal report file")

	stop := errors.New("stop")
	calls := 0
	err = WalkDir(fixtureDir, func(string, Report) error {
		calls++
		return stop
	})
	require.ErrorIs(t, err, stop)
	assert.Equal(t, 1, calls)
}

func TestReadDir(t *testing.T) {
	reports, err := ReadDir(fixtureDir)
	require.NoError(t, err)
	require.Len(t, reports, 2)
	assert.Equal(t, "calc/sub.go", reports[0].Source.Key())
	assert.Equal(t, "calc/add.go", reports[1].Source.Key())

	_, err = ReadDir(filepath.Join(t.TempDir(), "missing"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestWalkDir_SkipsReportsRemovedDuringTheWalk(t *testing.T) {
	dir := t.TempDir()

	for _, name := range []string{"a.yaml", "b.yaml", "c.yaml"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("result: []\n"), 0o600))
	}

	var seen []string

	err := WalkDir(dir, func(path string, _ Report) error {
		seen = append(seen, filepath.Base(path))
		if filepath.Base(path) == "a.yaml" {
			return os.Remove(filepath.Join(dir, "b.yaml"))
		}

		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"a.yaml", "c.yaml"}, seen)
}
