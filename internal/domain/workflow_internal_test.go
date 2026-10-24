package domain

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	m "gooze.dev/pkg/gooze/internal/model"
	pkg "gooze.dev/pkg/gooze/pkg"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

type failingCloseSpill struct {
	pkg.FileSpill[int]

	err error
}

func (s failingCloseSpill) Close() error { return s.err }

func TestCloseSpill_LogsOnlyAFailure(t *testing.T) {
	var logs bytes.Buffer

	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	closeSpill[int]("ok", failingCloseSpill{})
	assert.Empty(t, logs.String())

	closeSpill[int]("broken", failingCloseSpill{err: errors.New("disk")})
	assert.Contains(t, logs.String(), "Failed to close spill")
	assert.Contains(t, logs.String(), "spill=broken")
}

func TestSourceFileExists(t *testing.T) {
	file := filepath.Join(t.TempDir(), "a.go")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	assert.False(t, sourceFileExists(rpt.Source{}), "no origin")
	assert.True(t, sourceFileExists(rpt.Source{Origin: &rpt.File{FullPath: file}}))
	assert.False(t, sourceFileExists(rpt.Source{Origin: &rpt.File{FullPath: file + ".missing"}}))
}

func TestShardSourcesInCheckout_OnlyProjectRelativeSourcesCount(t *testing.T) {
	file := filepath.Join(t.TempDir(), "a.go")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	reports, err := pkg.NewFileSpill[m.Report]()
	require.NoError(t, err)

	defer reports.Close()

	require.NoError(t, reports.Append(m.Report{Source: rpt.Source{Origin: &rpt.File{FullPath: file}}}))

	inCheckout, err := shardSourcesInCheckout(reports, func(rpt.Source) error { return nil })
	require.NoError(t, err)
	assert.False(t, inCheckout, "a source without a short path proves nothing about the checkout")

	t.Chdir(filepath.Dir(file))
	require.NoError(t, os.WriteFile("b", nil, 0o600))
	require.NoError(t, reports.Append(m.Report{Source: rpt.Source{Origin: &rpt.File{ShortPath: "b"}}}))

	inCheckout, err = shardSourcesInCheckout(reports, func(rpt.Source) error { return nil })
	require.NoError(t, err)
	assert.True(t, inCheckout)
}

func TestFileKey(t *testing.T) {
	key, path := fileKey(m.Mutation{})
	assert.Empty(t, key)
	assert.Empty(t, path)

	key, path = fileKey(m.Mutation{Source: rpt.Source{Origin: &rpt.File{ShortPath: "a.go", Hash: "h1"}}})
	assert.Equal(t, "h1", key)
	assert.Equal(t, "a.go", path)

	key, _ = fileKey(m.Mutation{Source: rpt.Source{Origin: &rpt.File{ShortPath: "a.go"}}})
	assert.Equal(t, "a.go", key, "without a hash the path is the key")
}

func TestEstimateTally_SortsByPathThenCount(t *testing.T) {
	tally := newEstimateTally(Selection{})
	source := func(path, hash string) m.Mutation {
		return m.Mutation{Source: rpt.Source{Origin: &rpt.File{ShortPath: path, Hash: hash}}}
	}

	tally.add(source("b.go", "h1"), false)
	tally.add(source("b.go", "h1"), false)
	tally.add(source("b.go", "h2"), false)
	tally.add(source("a.go", "h3"), false)

	var got []string
	for _, file := range tally.estimation().Files {
		got = append(got, file.Path+":"+string(rune('0'+file.Count)))
	}

	assert.Equal(t, []string{"a.go:1", "b.go:1", "b.go:2"}, got)
}

func TestGetMutationStatus(t *testing.T) {
	mutation := m.Mutation{ID: "a", Type: mgen.Arithmetic}

	assert.Equal(t, rpt.Error, getMutationStatus(nil, mutation))
	assert.Equal(t, rpt.Error, getMutationStatus(m.Result{mgen.Arithmetic: {{MutationID: "b", Status: rpt.Killed}}}, mutation))
	assert.Equal(t, rpt.Survived, getMutationStatus(m.Result{mgen.Arithmetic: {
		{MutationID: "z", Status: rpt.Killed},
		{MutationID: "a", Status: rpt.Survived},
	}}, mutation))
}

// waitingWorkspace kills a mutant after delay, unless its context ends first.
type waitingWorkspace struct{ delay time.Duration }

func (ws waitingWorkspace) Run(ctx context.Context, mutation m.Mutation) (m.Result, error) {
	status := rpt.Killed

	select {
	case <-ctx.Done():
		status = rpt.Timeout
	case <-time.After(ws.delay):
	}

	return m.Result{mutation.Type: {{MutationID: mutation.ID, Status: status}}}, nil
}

func (waitingWorkspace) Close(context.Context) {}

func TestRunWithin(t *testing.T) {
	mutation := m.Mutation{ID: "a", Type: mgen.Arithmetic}
	ws := waitingWorkspace{delay: 50 * time.Millisecond}

	for timeout, want := range map[time.Duration]rpt.TestStatus{
		0:           rpt.Killed,
		-time.Hour:  rpt.Killed,
		1:           rpt.Timeout,
		time.Minute: rpt.Killed,
	} {
		result, err := runWithin(context.Background(), ws, mutation, timeout)
		require.NoError(t, err)
		assert.Equal(t, want, getMutationStatus(result, mutation), "timeout %s", timeout)
	}
}

func TestDispatch(t *testing.T) {
	mutation := m.Mutation{ID: "a"}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	require.ErrorIs(t, dispatch(canceled, nil, mutation), errNoWorkers)

	queue := make(chan m.Mutation, 1)
	require.NoError(t, dispatch(context.Background(), []chan m.Mutation{queue}, mutation))
	assert.Equal(t, mutation, <-queue)

	require.ErrorIs(t, dispatch(canceled, []chan m.Mutation{make(chan m.Mutation)}, mutation), context.Canceled)
}

func TestMerge_NeedsAReportsDir(t *testing.T) {
	err := NewWorkflow(nil, nil, nil, nil, nil).Merge(context.Background(), MergeArgs{})
	require.EqualError(t, err, "reports directory path is required")
}
