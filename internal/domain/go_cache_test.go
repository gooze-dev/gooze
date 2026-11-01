package domain_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gooze.dev/pkg/gooze/internal/adapter"
	domain "gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
	m "gooze.dev/pkg/gooze/internal/model"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// recordingBuildCache records the calls of a run, in order, with the mutants
// the run's workspace tested.
type recordingBuildCache struct {
	mu         sync.Mutex
	calls      []string
	sizeMB     int
	dirs       []string
	builds     []string // the environment that reached the runner
	env        []string
	prepareErr error
	prewarmErr error
	closeErr   error
}

func newRecordingBuildCache() *recordingBuildCache {
	return &recordingBuildCache{env: []string{"GOCACHEPROG=x"}}
}

func (c *recordingBuildCache) record(call string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.calls = append(c.calls, call)
}

func (c *recordingBuildCache) Prepare(_ context.Context, sizeMB int) ([]string, error) {
	c.record("prepare")
	c.sizeMB = sizeMB

	return c.env, c.prepareErr
}

func (c *recordingBuildCache) Prewarm(_ context.Context, dirs []string) error {
	c.record("prewarm")
	c.dirs = dirs

	return c.prewarmErr
}

func (c *recordingBuildCache) SetBuildEnv(env []string) {
	c.record("env")
	c.builds = env
}

func (c *recordingBuildCache) Close(ctx context.Context) error {
	if ctx.Err() != nil {
		c.record("close on a canceled context")
	}

	c.record("close")

	return c.closeErr
}

type recordingWorkspace struct {
	cache  *recordingBuildCache
	cancel context.CancelFunc
}

func (ws recordingWorkspace) Run(_ context.Context, mutation m.Mutation) (m.Result, error) {
	ws.cache.record("mutant")

	if ws.cancel != nil {
		ws.cancel()
	}

	return m.Result{mutation.Type: {{MutationID: mutation.ID, Status: rpt.Killed}}}, nil
}

func (recordingWorkspace) Close(context.Context) {}

// runWithBuildCache tests two sources of /p, one of them without tests, and
// one of /q, with cache, under mode.
func runWithBuildCache(t *testing.T, cache *recordingBuildCache, mode domain.GoCacheMode, interrupt bool) error {
	t.Helper()

	return runSourcesWithBuildCache(t, cache, mode, interrupt, []rpt.Source{
		{Origin: &rpt.File{ShortPath: "p/a.go", FullPath: "/p/a.go", Hash: "a"}, Test: &rpt.File{ShortPath: "p/a_test.go", FullPath: "/p/a_test.go", Hash: "at"}},
		{Origin: &rpt.File{ShortPath: "p/b.go", FullPath: "/p/b.go", Hash: "b"}, Test: &rpt.File{ShortPath: "p/b_test.go", FullPath: "/p/b_test.go", Hash: "bt"}},
		{Origin: &rpt.File{ShortPath: "q/c.go", FullPath: "/q/c.go", Hash: "c"}, Test: &rpt.File{ShortPath: "q/c_test.go", FullPath: "/q/c_test.go", Hash: "ct"}},
		{Origin: &rpt.File{ShortPath: "r/d.go", FullPath: "/r/d.go", Hash: "d"}},
	})
}

func runSourcesWithBuildCache(t *testing.T, cache *recordingBuildCache, mode domain.GoCacheMode, interrupt bool, sources []rpt.Source) error {
	t.Helper()

	fs := domainmocks.NewMockSourceFSAdapter(t)
	fs.EXPECT().Stream(mock.Anything, mock.Anything).Return(streamSources(sources))

	mutagen := domainmocks.NewMockMutagen(t)
	mutagen.EXPECT().StreamMutations(mock.Anything, mock.Anything, mock.Anything, typeArgs(domain.DefaultMutations)...).
		RunAndReturn(func(_ context.Context, source rpt.Source, fn func(m.Mutation) error, _ ...mgen.Type) error {
			if source.Test == nil {
				return nil
			}

			return fn(m.Mutation{ID: source.Key(), Source: source, Type: mgen.Arithmetic, Line: 1})
		}).Maybe()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ws := recordingWorkspace{cache: cache}
	if interrupt {
		ws.cancel = cancel
	}

	orchestrator := domainmocks.NewMockOrchestrator(t)
	orchestrator.EXPECT().NewWorkspace(mock.Anything).Return(ws).Maybe()

	reporter := domainmocks.NewMockReporter(t)
	reporter.EXPECT().StartTest(mock.Anything).Return(nil)
	reporter.EXPECT().Wait(mock.Anything).Return().Maybe()
	reporter.EXPECT().Close(mock.Anything).Return()
	reporter.EXPECT().DisplayConcurrencyInfo(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	reporter.EXPECT().DisplayUpcomingTestsInfo(mock.Anything, mock.Anything).Return().Maybe()
	reporter.EXPECT().DisplayStartingTestInfo(mock.Anything, mock.Anything, mock.Anything).Return().Maybe()
	reporter.EXPECT().DisplayCompletedTestInfo(mock.Anything, mock.Anything, mock.Anything).Return().Maybe()
	reporter.EXPECT().DisplayMutationScore(mock.Anything, mock.Anything).Return().Maybe()

	args := domain.TestArgs{
		EstimateArgs:    domain.EstimateArgs{Paths: []m.Path{"./..."}, Reports: m.Path(t.TempDir())},
		Threads:         1,
		TotalShardCount: 1,
		GoCache:         domain.GoCache{Mode: mode, SizeMB: 7, Prewarm: true},
	}
	args.Reports = args.EstimateArgs.Reports

	wf := domain.NewWorkflow(fs, adapter.NewReportStore(), reporter, orchestrator, mutagen, domain.WithBuildCache(cache, cache))

	return wf.Test(ctx, args)
}

func TestWorkflow_Test_IsolatedGoCache(t *testing.T) {
	cache := newRecordingBuildCache()

	require.NoError(t, runWithBuildCache(t, cache, domain.GoCacheIsolated, false))
	assert.Equal(t, []string{"prepare", "prewarm", "env", "mutant", "mutant", "mutant", "close"}, cache.calls)
	assert.Equal(t, 7, cache.sizeMB)
	assert.Equal(t, []string{"GOCACHEPROG=x"}, cache.builds)
	assert.Equal(t, []string{"/p", "/q"}, cache.dirs, "the packages with tests, once each")
}

func TestWorkflow_Test_GoCacheOtherModesLeaveItAlone(t *testing.T) {
	for _, mode := range []domain.GoCacheMode{domain.GoCacheShared, ""} {
		cache := newRecordingBuildCache()

		require.NoError(t, runWithBuildCache(t, cache, mode, false))
		assert.Equal(t, []string{"mutant", "mutant", "mutant"}, cache.calls, string(mode))
	}
}

func TestWorkflow_Test_GoCacheUnsupportedSkipsPrewarm(t *testing.T) {
	cache := &recordingBuildCache{}

	require.NoError(t, runWithBuildCache(t, cache, domain.GoCacheIsolated, false))
	assert.Equal(t, []string{"prepare", "env", "mutant", "mutant", "mutant", "close"}, cache.calls)
}

func TestWorkflow_Test_GoCacheNoPrewarmWithoutPackages(t *testing.T) {
	cache := newRecordingBuildCache()
	sources := []rpt.Source{{Origin: &rpt.File{ShortPath: "r/d.go", FullPath: "/r/d.go", Hash: "d"}}}

	require.NoError(t, runSourcesWithBuildCache(t, cache, domain.GoCacheIsolated, false, sources))
	assert.NotContains(t, cache.calls, "prewarm")
	assert.Contains(t, cache.calls, "close")
}

func TestWorkflow_Test_GoCachePrepareFails(t *testing.T) {
	cache := &recordingBuildCache{prepareErr: errors.New("no go")}

	err := runWithBuildCache(t, cache, domain.GoCacheIsolated, false)
	require.ErrorIs(t, err, cache.prepareErr)
	assert.Equal(t, []string{"prepare"}, cache.calls)
}

func TestWorkflow_Test_GoCachePrewarmFailureOnlyWarns(t *testing.T) {
	logs := captureLogs(t)
	cache := newRecordingBuildCache()

	require.NoError(t, runWithBuildCache(t, cache, domain.GoCacheIsolated, false))
	assert.NotContains(t, logs.String(), "Failed to prewarm")

	cache = newRecordingBuildCache()
	cache.prewarmErr = errors.New("broken package")

	require.NoError(t, runWithBuildCache(t, cache, domain.GoCacheIsolated, false))
	assert.Contains(t, logs.String(), "Failed to prewarm")
	assert.Contains(t, cache.calls, "mutant")
}

func TestWorkflow_Test_GoCacheCloseFailureOnlyWarns(t *testing.T) {
	logs := captureLogs(t)
	cache := newRecordingBuildCache()

	require.NoError(t, runWithBuildCache(t, cache, domain.GoCacheIsolated, false))
	assert.NotContains(t, logs.String(), "Failed to trim")

	cache = newRecordingBuildCache()
	cache.closeErr = errors.New("trim")

	require.NoError(t, runWithBuildCache(t, cache, domain.GoCacheIsolated, false))
	assert.Equal(t, "close", cache.calls[len(cache.calls)-1])
	assert.Contains(t, logs.String(), "Failed to trim")
}

func TestWorkflow_Test_GoCacheClosedOnInterrupt(t *testing.T) {
	cache := newRecordingBuildCache()

	require.ErrorIs(t, runWithBuildCache(t, cache, domain.GoCacheIsolated, true), context.Canceled)
	assert.Equal(t, "close", cache.calls[len(cache.calls)-1], "no cancellation reaches the trim")
	assert.False(t, slices.Contains(cache.calls, "close on a canceled context"))
}
