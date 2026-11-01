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
	settings   domain.GoCache
	dirs       []string
	prepareErr error
	closeErr   error
}

func (c *recordingBuildCache) record(call string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.calls = append(c.calls, call)
}

func (c *recordingBuildCache) Prepare(_ context.Context, settings domain.GoCache, dirs []string) error {
	c.record("prepare")
	c.settings, c.dirs = settings, dirs

	return c.prepareErr
}

func (c *recordingBuildCache) Close(ctx context.Context) error {
	c.record("close")

	if ctx.Err() != nil {
		c.record("close on a canceled context")
	}

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

	sources := []rpt.Source{
		{Origin: &rpt.File{ShortPath: "p/a.go", FullPath: "/p/a.go", Hash: "a"}, Test: &rpt.File{ShortPath: "p/a_test.go", FullPath: "/p/a_test.go", Hash: "at"}},
		{Origin: &rpt.File{ShortPath: "p/b.go", FullPath: "/p/b.go", Hash: "b"}, Test: &rpt.File{ShortPath: "p/b_test.go", FullPath: "/p/b_test.go", Hash: "bt"}},
		{Origin: &rpt.File{ShortPath: "q/c.go", FullPath: "/q/c.go", Hash: "c"}, Test: &rpt.File{ShortPath: "q/c_test.go", FullPath: "/q/c_test.go", Hash: "ct"}},
		{Origin: &rpt.File{ShortPath: "r/d.go", FullPath: "/r/d.go", Hash: "d"}},
	}

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

	wf := domain.NewWorkflow(fs, adapter.NewReportStore(), reporter, orchestrator, mutagen, domain.WithBuildCache(cache))

	return wf.Test(ctx, args)
}

func TestWorkflow_Test_IsolatedGoCache(t *testing.T) {
	cache := &recordingBuildCache{}

	require.NoError(t, runWithBuildCache(t, cache, domain.GoCacheIsolated, false))
	assert.Equal(t, []string{"prepare", "mutant", "mutant", "mutant", "close"}, cache.calls)
	assert.Equal(t, domain.GoCache{Mode: domain.GoCacheIsolated, SizeMB: 7, Prewarm: true}, cache.settings)
	assert.Equal(t, []string{"/p", "/q"}, cache.dirs, "the packages with tests, once each")
}

func TestWorkflow_Test_SharedGoCache(t *testing.T) {
	cache := &recordingBuildCache{}

	require.NoError(t, runWithBuildCache(t, cache, domain.GoCacheShared, false))
	assert.Equal(t, []string{"mutant", "mutant", "mutant"}, cache.calls)
}

func TestWorkflow_Test_GoCachePrepareFails(t *testing.T) {
	cache := &recordingBuildCache{prepareErr: errors.New("no go")}

	err := runWithBuildCache(t, cache, domain.GoCacheIsolated, false)
	require.ErrorIs(t, err, cache.prepareErr)
	assert.Equal(t, []string{"prepare"}, cache.calls)
}

func TestWorkflow_Test_GoCacheCloseFails(t *testing.T) {
	cache := &recordingBuildCache{closeErr: errors.New("trim")}

	require.NoError(t, runWithBuildCache(t, cache, domain.GoCacheIsolated, false))
	assert.Equal(t, "close", cache.calls[len(cache.calls)-1])
}

func TestWorkflow_Test_GoCacheClosedOnInterrupt(t *testing.T) {
	cache := &recordingBuildCache{}

	require.ErrorIs(t, runWithBuildCache(t, cache, domain.GoCacheIsolated, true), context.Canceled)
	assert.Equal(t, "close", cache.calls[len(cache.calls)-1], "no cancellation reaches the trim")
	assert.False(t, slices.Contains(cache.calls, "close on a canceled context"))
}
