package domain_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	domain "gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// scriptedWorkspace gives each mutant the status in statuses, killed when it
// has none, and records the mutants it ran.
type scriptedWorkspace struct {
	mu       sync.Mutex
	statuses map[string]rpt.TestStatus
	ran      []string
}

func (ws *scriptedWorkspace) Run(_ context.Context, mutation m.Mutation) (m.Result, error) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	ws.ran = append(ws.ran, mutation.ID)

	status, ok := ws.statuses[mutation.ID]
	if !ok {
		status = rpt.Killed
	}

	return m.Result{mutation.Type: {{MutationID: mutation.ID, Status: status}}}, nil
}

func (ws *scriptedWorkspace) Close(context.Context) {}

// failFastRun tests the fixture (a-1..a-4, c-1, c-2, in that order) with
// policy and thresholds, the mutants in survivors surviving. It returns the
// run's error, the mutants it ran and the score it displayed.
func (fx interruptFixture) failFastRun(t *testing.T, policy domain.FailFast, thresholds domain.Thresholds, survivors ...string) (error, []string, domain.MutationScore) {
	t.Helper()

	statuses := map[string]rpt.TestStatus{}
	for _, id := range survivors {
		statuses[id] = rpt.Survived
	}

	ws := &scriptedWorkspace{statuses: statuses}

	args := cachedTestArgs(fx.reports)
	args.FailFast = policy
	args.Thresholds = thresholds

	err, score := fx.runWith(context.Background(), t, ws, args)

	return err, ws.ran, score
}

var total80 = domain.Thresholds{Key: "threshold", Total: 80}

func TestWorkflow_Test_FailFastOffTestsEverything(t *testing.T) {
	fx := newInterruptFixture(t)

	err, ran, score := fx.failFastRun(t, domain.FailFastOff, total80, "a-1", "a-2")

	require.EqualError(t, err, "mutation score 66.67% is below threshold.total 80%")
	assert.Equal(t, []string{"a-1", "a-2", "a-3", "a-4", "c-1", "c-2"}, ran)
	assert.Empty(t, score.FailFast)
}

func TestWorkflow_Test_FailFastFirstStopsAtTheFirstSurvivor(t *testing.T) {
	fx := newInterruptFixture(t)

	err, ran, score := fx.failFastRun(t, domain.FailFastFirst, domain.Thresholds{Key: "threshold"}, "a-2", "c-1")

	require.EqualError(t, err, "stopped at the first surviving mutant, a.go:2 (fail-fast first)")
	assert.Equal(t, m.ExitBelowThreshold, exitCodeOf(err))
	assert.Equal(t, []string{"a-1", "a-2"}, ran)
	// a.go is unfinished, so its results are marked as such and the next run
	// tests the rest; c.go was never started.
	assert.Equal(t, map[string]*rpt.Scope{"a-1": interrupted, "a-2": interrupted}, fx.stored(t))
	assert.Equal(t, domain.FailFastFirst, score.FailFast)
	assert.Equal(t, 2, score.Tested)
	assert.Equal(t, 6, score.Planned)
	assert.Equal(t, rpt.Summary{Total: 2, Killed: 1, Survived: 1}, score.Summary)
	assert.False(t, score.Interrupted)
}

func TestWorkflow_Test_FailFastFirstPrefersABreachedThreshold(t *testing.T) {
	fx := newInterruptFixture(t)

	err, _, _ := fx.failFastRun(t, domain.FailFastFirst, total80, "a-1")

	require.EqualError(t, err, "mutation score 0.00% is below threshold.total 80%")
	assert.Equal(t, m.ExitBelowThreshold, exitCodeOf(err))
}

func TestWorkflow_Test_RunAfterAFailFastStopTestsTheRest(t *testing.T) {
	fx := newInterruptFixture(t)

	_, _, _ = fx.failFastRun(t, domain.FailFastFirst, domain.Thresholds{}, "a-2")

	err, ran, score := fx.failFastRun(t, domain.FailFastOff, domain.Thresholds{}, "a-2")

	require.NoError(t, err)
	assert.Equal(t, []string{"a-3", "a-4", "c-1", "c-2"}, ran)
	assert.Equal(t, rpt.Summary{Total: 6, Killed: 5, Survived: 1}, score.Summary)
}

func TestWorkflow_Test_FailFastFirstWithoutSurvivorsTestsEverything(t *testing.T) {
	fx := newInterruptFixture(t)

	err, ran, score := fx.failFastRun(t, domain.FailFastFirst, domain.Thresholds{})

	require.NoError(t, err)
	assert.Len(t, ran, 6)
	assert.Empty(t, score.FailFast)
}

func TestWorkflow_Test_FailFastGateStopsOnceTheTotalIsOutOfReach(t *testing.T) {
	fx := newInterruptFixture(t)

	// After a-1 the best possible score is 5 of 6, 83.33%; after a-2 it is
	// 4 of 6, 66.67%, below 80%.
	err, ran, score := fx.failFastRun(t, domain.FailFastGate, total80, "a-1", "a-2")

	require.EqualError(t, err, "mutation score 0.00% is below threshold.total 80%")
	assert.Equal(t, m.ExitBelowThreshold, exitCodeOf(err))
	assert.Equal(t, []string{"a-1", "a-2"}, ran)
	assert.Equal(t, domain.FailFastGate, score.FailFast)
	assert.Equal(t, 2, score.Tested)
	assert.Equal(t, 6, score.Planned)
}

func TestWorkflow_Test_FailFastGateKeepsGoingWhileTheGateCanBeMet(t *testing.T) {
	fx := newInterruptFixture(t)

	// One survivor of six still allows 83.33%.
	err, ran, score := fx.failFastRun(t, domain.FailFastGate, total80, "a-1")

	require.NoError(t, err)
	assert.Len(t, ran, 6)
	assert.Empty(t, score.FailFast)
}

func TestWorkflow_Test_FailFastGateStopsAboveMaxSurvivors(t *testing.T) {
	fx := newInterruptFixture(t)

	err, ran, _ := fx.failFastRun(t, domain.FailFastGate, domain.Thresholds{Key: "threshold", MaxSurvivors: 1}, "a-1", "c-1")

	require.EqualError(t, err, "2 surviving mutants are above threshold.max_survivors 1")
	assert.Equal(t, []string{"a-1", "a-2", "a-3", "a-4", "c-1"}, ran)
}

func TestWorkflow_Test_FailFastGateStopsOnceAFileIsOutOfReach(t *testing.T) {
	fx := newInterruptFixture(t)

	// c.go can reach at most 50% once c-1 survives.
	err, ran, _ := fx.failFastRun(t, domain.FailFastGate, domain.Thresholds{Key: "threshold", File: 60}, "c-1")

	require.EqualError(t, err, "c.go: score 0.00% is below threshold.file 60%")
	assert.Equal(t, []string{"a-1", "a-2", "a-3", "a-4", "c-1"}, ran)
}

func TestWorkflow_Test_FailFastGateWithoutGatesTestsEverything(t *testing.T) {
	fx := newInterruptFixture(t)

	err, ran, _ := fx.failFastRun(t, domain.FailFastGate, domain.Thresholds{}, "a-1", "a-2", "a-3")

	require.NoError(t, err)
	assert.Len(t, ran, 6)
}

func TestWorkflow_Test_FailFastGateCountsCachedResults(t *testing.T) {
	fx := newInterruptFixture(t)

	// a.go's four killed mutants are cached; c.go changes and is tested again.
	_, _, _ = fx.failFastRun(t, domain.FailFastOff, domain.Thresholds{})
	fx.sources[1].Origin.Hash = "c2"

	// c-1 surviving leaves c.go at 50% at best, but the total can still
	// reach 5 of 6, 83.33%, with the cached results.
	err, ran, score := fx.failFastRun(t, domain.FailFastGate, total80, "c-1")

	require.NoError(t, err)
	assert.Equal(t, []string{"c-1", "c-2"}, ran)
	assert.Equal(t, rpt.Summary{Total: 6, Killed: 5, Survived: 1}, score.Summary)
}

func TestWorkflow_Test_FailFastFileSkipsTheRestOfAFailedFile(t *testing.T) {
	fx := newInterruptFixture(t)

	// a.go reaches at most 50% after a-1 and a-2 survive, below 75%: its
	// other mutants are skipped, and c.go is still tested.
	err, ran, score := fx.failFastRun(t, domain.FailFastFile, domain.Thresholds{Key: "threshold", File: 75}, "a-1", "a-2")

	require.EqualError(t, err, "a.go: score 0.00% is below threshold.file 75%")
	assert.Equal(t, m.ExitBelowThreshold, exitCodeOf(err))
	assert.Equal(t, []string{"a-1", "a-2", "c-1", "c-2"}, ran)
	assert.Equal(t, map[string]*rpt.Scope{"a-1": interrupted, "a-2": interrupted, "c-1": nil, "c-2": nil}, fx.stored(t))
	assert.Equal(t, domain.FailFastFile, score.FailFast)
	assert.Equal(t, 4, score.Tested)
	assert.Equal(t, 6, score.Planned)
}

func TestWorkflow_Test_FailFastFileWithoutAFileThresholdTestsEverything(t *testing.T) {
	fx := newInterruptFixture(t)

	err, ran, _ := fx.failFastRun(t, domain.FailFastFile, total80, "a-1", "a-2")

	require.EqualError(t, err, "mutation score 66.67% is below threshold.total 80%")
	assert.Len(t, ran, 6)
}

func TestWorkflow_Test_FailFastWithoutTheCache(t *testing.T) {
	fx := newInterruptFixture(t)

	ws := &scriptedWorkspace{statuses: map[string]rpt.TestStatus{"a-2": rpt.Survived}}
	args := cachedTestArgs(fx.reports)
	args.UseCache = false
	args.FailFast = domain.FailFastFirst

	err, score := fx.runWith(context.Background(), t, ws, args)

	require.EqualError(t, err, "stopped at the first surviving mutant, a.go:2 (fail-fast first)")
	assert.Equal(t, []string{"a-1", "a-2"}, ws.ran)
	assert.Equal(t, 6, score.Planned)
}

func TestWorkflow_Test_FailFastFileSkipsTheLastMutantOfAFailedFile(t *testing.T) {
	fx := newInterruptFixture(t)

	// a.go reaches at most 75% once a-3 survives, below 80%, with only a-4
	// left.
	err, ran, _ := fx.failFastRun(t, domain.FailFastFile, domain.Thresholds{Key: "threshold", File: 80}, "a-3")

	require.EqualError(t, err, "a.go: score 66.67% is below threshold.file 80%")
	assert.Equal(t, []string{"a-1", "a-2", "a-3", "c-1", "c-2"}, ran)
}

func TestWorkflow_Test_FailFastGateOnAShardChecksOnlyTheSurvivorCap(t *testing.T) {
	fx := newInterruptFixture(t)

	statuses := map[string]rpt.TestStatus{}
	for _, mutations := range fx.mutations {
		for _, mutation := range mutations {
			statuses[mutation.ID] = rpt.Survived
		}
	}

	var ran []string

	for index := range 2 {
		ws := &scriptedWorkspace{statuses: statuses}
		args := cachedTestArgs(fx.reports)
		args.ShardIndex, args.TotalShardCount = index, 2
		args.FailFast = domain.FailFastGate
		args.Thresholds = total80

		// A shard's score says nothing of the merged one, so every mutant
		// is tested and the total is left to report merge.
		err, _ := fx.runWith(context.Background(), t, ws, args)
		require.NoError(t, err)

		ran = append(ran, ws.ran...)
	}

	assert.ElementsMatch(t, []string{"a-1", "a-2", "a-3", "a-4", "c-1", "c-2"}, ran)
}

func TestWorkflow_Test_FailFastGateCountsResumedResults(t *testing.T) {
	fx := newInterruptFixture(t)

	// a-1 and a-2 are killed, then the run is interrupted in a-3.
	_, _, _ = fx.run(t, 2)

	// With a-1 and a-2 resumed, a.go and c.go can still reach 5 of 6 after
	// a-3 survives, and 4 of 6, below 80%, after a-4 does.
	err, ran, _ := fx.failFastRun(t, domain.FailFastGate, total80, "a-3", "a-4")

	require.EqualError(t, err, "mutation score 50.00% is below threshold.total 80%")
	assert.Equal(t, []string{"a-3", "a-4"}, ran)
}

func TestWorkflow_Test_FailFastGateCountsNotCoveredMutants(t *testing.T) {
	fx := newInterruptFixture(t)

	ws := &scriptedWorkspace{}
	args := cachedTestArgs(fx.reports)
	args.CoverageProfile = "cov.out"
	args.FailFast = domain.FailFastGate
	args.Thresholds = total80

	// a-1 and a-2 are not covered, so 4 of 6, below 80%, is the best left:
	// nothing needs to run.
	err, _ := fx.runWith(context.Background(), t, ws, args)

	require.EqualError(t, err, "mutation score 0.00% is below threshold.total 80%")
	assert.Empty(t, ws.ran)
}
