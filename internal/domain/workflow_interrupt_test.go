package domain_test

import (
	"context"
	"path/filepath"
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

// interruptingWorkspace kills every mutant it runs. Once it has run after
// mutants, it cancels the run while the next one is in flight, which then
// ends like a canceled go test: as a timeout.
type interruptingWorkspace struct {
	mu     sync.Mutex
	after  int // finished mutants before the interruption; < 0 never interrupts
	cancel context.CancelFunc
	ran    []string
}

func (ws *interruptingWorkspace) Run(_ context.Context, mutation m.Mutation) (m.Result, error) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	ws.ran = append(ws.ran, mutation.ID)

	if len(ws.ran) == ws.after+1 {
		ws.cancel()
		return m.Result{mutation.Type: {{MutationID: mutation.ID, Status: rpt.Timeout}}}, nil
	}

	return m.Result{mutation.Type: {{MutationID: mutation.ID, Status: rpt.Killed}}}, nil
}

func (ws *interruptingWorkspace) Close(context.Context) {}

// interruptFixture is a project of a.go (four mutants) and c.go (two).
type interruptFixture struct {
	reports   string
	sources   []rpt.Source
	mutations map[string][]m.Mutation
}

func newInterruptFixture(t *testing.T) interruptFixture {
	t.Helper()

	a := rpt.Source{Origin: &rpt.File{ShortPath: "a.go", FullPath: "/p/a.go", Hash: "a1"}, Test: &rpt.File{ShortPath: "a_test.go", FullPath: "/p/a_test.go", Hash: "at"}}
	c := rpt.Source{Origin: &rpt.File{ShortPath: "c.go", FullPath: "/p/c.go", Hash: "c1"}, Test: &rpt.File{ShortPath: "c_test.go", FullPath: "/p/c_test.go", Hash: "ct"}}

	mutations := map[string][]m.Mutation{}
	for _, source := range []rpt.Source{a, c} {
		count := map[string]int{"a.go": 4, "c.go": 2}[source.Key()]
		for i := range count {
			id := source.Key()[:1] + "-" + string(rune('1'+i))
			mutations[source.Key()] = append(mutations[source.Key()], m.Mutation{ID: id, Source: source, Type: mgen.Arithmetic, Line: i + 1})
		}
	}

	return interruptFixture{reports: filepath.Join(t.TempDir(), "reports"), sources: []rpt.Source{a, c}, mutations: mutations}
}

// run tests the fixture with the reports cache on, interrupting after
// finished mutants (< 0: never). It returns the run's error, the mutants it
// ran and the score it displayed.
func (fx interruptFixture) run(t *testing.T, after int) (error, []string, domain.MutationScore) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ws := &interruptingWorkspace{after: after, cancel: cancel}
	err, score := fx.runWith(ctx, t, ws, cachedTestArgs(fx.reports))

	return err, ws.ran, score
}

// runWith tests the fixture in ws with args and returns the run's error and
// the score it displayed.
func (fx interruptFixture) runWith(ctx context.Context, t *testing.T, ws domain.Workspace, args domain.TestArgs) (error, domain.MutationScore) {
	t.Helper()

	fs := domainmocks.NewMockSourceFSAdapter(t)
	fs.EXPECT().Stream(mock.Anything, mock.Anything).Return(streamSources(fx.sources))

	if args.CoverageProfile != "" {
		fs.EXPECT().ReadFile(mock.Anything, args.CoverageProfile).Return(fixtureCoverage, nil)
	}

	stream := func(_ context.Context, source rpt.Source, fn func(m.Mutation) error, types ...mgen.Type) error {
		if !slices.Contains(types, mgen.Arithmetic) {
			return nil
		}

		for _, mutation := range fx.mutations[source.Key()] {
			if err := fn(mutation); err != nil {
				return err
			}
		}

		return nil
	}

	// The run generates every default mutagen; the cache asks a complete
	// file about the ones it has no results of (all but arithmetic).
	mutagen := domainmocks.NewMockMutagen(t)
	mutagen.EXPECT().StreamMutations(mock.Anything, mock.Anything, mock.Anything, typeArgs(domain.DefaultMutations)...).
		RunAndReturn(stream).Maybe()
	mutagen.EXPECT().StreamMutations(mock.Anything, mock.Anything, mock.Anything, typeArgs(domain.DefaultMutations[1:])...).
		RunAndReturn(stream).Maybe()

	orchestrator := domainmocks.NewMockOrchestrator(t)
	orchestrator.EXPECT().NewWorkspace(mock.Anything).Return(ws).Maybe()

	var score domain.MutationScore

	reporter := domainmocks.NewMockReporter(t)
	reporter.EXPECT().StartTest(mock.Anything).Return(nil)
	reporter.EXPECT().Wait(mock.Anything).Return().Maybe()
	reporter.EXPECT().Close(mock.Anything).Return()
	reporter.EXPECT().DisplayConcurrencyInfo(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	reporter.EXPECT().DisplayUpcomingTestsInfo(mock.Anything, mock.Anything).Return()
	reporter.EXPECT().DisplayStartingTestInfo(mock.Anything, mock.Anything, mock.Anything).Return().Maybe()
	reporter.EXPECT().DisplayCompletedTestInfo(mock.Anything, mock.Anything, mock.Anything).Return().Maybe()
	reporter.EXPECT().DisplayMutationScore(mock.Anything, mock.Anything).
		Run(func(_ context.Context, s domain.MutationScore) { score = s }).Maybe()

	wf := domain.NewWorkflow(fs, adapter.NewReportStore(), reporter, orchestrator, mutagen)
	err := wf.Test(ctx, args)

	return err, score
}

// stored returns the scope of each stored mutation, by mutation ID.
func (fx interruptFixture) stored(t *testing.T) map[string]*rpt.Scope {
	t.Helper()

	reports, err := adapter.NewReportStore().LoadSpillReports(context.Background(), m.Path(fx.reports))
	require.NoError(t, err)

	defer reports.Close()

	scopes := map[string]*rpt.Scope{}

	require.NoError(t, reports.Range(func(_ uint64, report m.Report) error {
		for _, results := range report.Result {
			for _, result := range results {
				scopes[result.MutationID] = report.Scope
			}
		}

		return nil
	}))

	return scopes
}

var interrupted = &rpt.Scope{Interrupted: true}

// fixtureCoverage is a coverage profile of the fixture in which no test runs
// lines 1 and 2 of a.go, the lines of a-1 and a-2.
var fixtureCoverage = []byte("mode: set\nmod/a.go:1.1,2.20 1 0\nmod/a.go:3.1,4.20 1 1\nmod/c.go:1.1,2.20 1 1\n")

func TestWorkflow_Test_InterruptedRunSavesFinishedMutants(t *testing.T) {
	fx := newInterruptFixture(t)

	err, ran, score := fx.run(t, 2)

	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, []string{"a-1", "a-2", "a-3"}, ran)
	// a-3 was in flight: it is not saved. a.go is incomplete, so its results
	// are marked as such.
	assert.Equal(t, map[string]*rpt.Scope{"a-1": interrupted, "a-2": interrupted}, fx.stored(t))
	assert.True(t, score.Interrupted)
	assert.Equal(t, 2, score.Tested)
	assert.Equal(t, 6, score.Planned)
	assert.Equal(t, rpt.Summary{Total: 2, Killed: 2}, score.Summary)
}

func TestWorkflow_Test_RunAfterAnInterruptionTestsOnlyTheMissingMutants(t *testing.T) {
	fx := newInterruptFixture(t)

	_, _, _ = fx.run(t, 2)

	err, ran, score := fx.run(t, -1)

	require.NoError(t, err)
	assert.Equal(t, []string{"a-3", "a-4", "c-1", "c-2"}, ran)
	assert.Equal(t, map[string]*rpt.Scope{"a-1": nil, "a-2": nil, "a-3": nil, "a-4": nil, "c-1": nil, "c-2": nil}, fx.stored(t),
		"a.go is complete again")
	assert.False(t, score.Interrupted)
	assert.Equal(t, 4, score.Tested)
	assert.Equal(t, 2, score.Cached)
	assert.Equal(t, rpt.Summary{Total: 6, Killed: 6}, score.Summary)
}

func TestWorkflow_Test_ACompletedFileOfAnInterruptedRunIsCached(t *testing.T) {
	fx := newInterruptFixture(t)

	// a.go finishes, c-1 is in flight.
	_, _, _ = fx.run(t, 4)
	assert.Equal(t, map[string]*rpt.Scope{"a-1": nil, "a-2": nil, "a-3": nil, "a-4": nil}, fx.stored(t))

	err, ran, _ := fx.run(t, -1)

	require.NoError(t, err)
	assert.Equal(t, []string{"c-1", "c-2"}, ran)
}

func TestWorkflow_Test_InterruptedResumeKeepsEarlierResults(t *testing.T) {
	fx := newInterruptFixture(t)

	_, _, _ = fx.run(t, 1)
	// The second run tests a-2 and a-3, then is interrupted in a-4.
	err, ran, score := fx.run(t, 2)

	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, []string{"a-2", "a-3", "a-4"}, ran)
	assert.Equal(t, map[string]*rpt.Scope{"a-1": interrupted, "a-2": interrupted, "a-3": interrupted}, fx.stored(t))
	assert.Equal(t, 2, score.Tested)
	assert.Equal(t, 5, score.Planned)

	err, ran, _ = fx.run(t, -1)

	require.NoError(t, err)
	assert.Equal(t, []string{"a-4", "c-1", "c-2"}, ran)
}

func TestWorkflow_Test_ChangedFileAfterAnInterruptionIsTestedInFull(t *testing.T) {
	fx := newInterruptFixture(t)

	_, _, _ = fx.run(t, 2)

	fx.sources[0].Origin.Hash = "a2"

	err, ran, _ := fx.run(t, -1)

	require.NoError(t, err)
	assert.Equal(t, []string{"a-1", "a-2", "a-3", "a-4", "c-1", "c-2"}, ran)
}

func TestWorkflow_Test_ChangedTestAfterAnInterruptionIsTestedInFull(t *testing.T) {
	fx := newInterruptFixture(t)

	_, _, _ = fx.run(t, 2)

	fx.sources[0].Test.Hash = "at2"

	_, ran, _ := fx.run(t, -1)

	assert.Equal(t, []string{"a-1", "a-2", "a-3", "a-4", "c-1", "c-2"}, ran)
}

func TestWorkflow_Test_InterruptionBeforeAnyResultSavesNothing(t *testing.T) {
	fx := newInterruptFixture(t)

	err, ran, score := fx.run(t, 0)

	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, []string{"a-1"}, ran)
	assert.Empty(t, fx.stored(t))
	assert.True(t, score.Interrupted)
	assert.Equal(t, 0, score.Tested)
	assert.Equal(t, 6, score.Planned)
}

func TestWorkflow_Test_UninterruptedRunSkipsNothing(t *testing.T) {
	fx := newInterruptFixture(t)

	err, ran, score := fx.run(t, -1)

	require.NoError(t, err)
	assert.Len(t, ran, 6)
	assert.Len(t, fx.stored(t), 6)
	assert.False(t, score.Interrupted)
	assert.Zero(t, score.Planned, "only an interrupted run says how many mutants it was to test")
}
