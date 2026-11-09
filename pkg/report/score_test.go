package report

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestScore(t *testing.T) {
	assert.InDelta(t, 1.0, Score(0, 0), 0)
	assert.InDelta(t, 0.0, Score(0, 4), 0)
	assert.InDelta(t, 0.5, Score(1, 2), 0)
	assert.InDelta(t, 0.75, Score(3, 4), 0)
}

func TestSummary_Add(t *testing.T) {
	var s Summary

	for _, status := range []TestStatus{Killed, Killed, Survived, Skipped, Timeout, Error, NotCovered, TestStatus(9)} {
		s.Add(status)
	}

	assert.Equal(t, Summary{Total: 8, Killed: 2, Survived: 1, Skipped: 1, Timeout: 1, Error: 1, NotCovered: 1}, s)
}

func TestSummary_ScoreExcludesErrors(t *testing.T) {
	s := Summary{Total: 4, Killed: 1, Error: 2, Survived: 1}

	assert.Equal(t, 2, s.Scored())
	assert.InDelta(t, 0.5, s.Score(), 0)
}

func TestSummary_ScoreCountsNotCoveredAsNotKilled(t *testing.T) {
	s := Summary{Total: 4, Killed: 2, NotCovered: 2}

	assert.Equal(t, 4, s.Scored())
	assert.InDelta(t, 0.5, s.Score(), 0)
}

func TestSummary_EmptyAndAllErrorsScoreFull(t *testing.T) {
	assert.InDelta(t, 1.0, Summary{}.Score(), 0)
	assert.InDelta(t, 1.0, Summary{Total: 2, Error: 2}.Score(), 0)
}

func TestSummarize(t *testing.T) {
	r := Report{Result: []Result{
		{Name: "arithmetic", Mutations: []MutationResult{{Status: Killed}, {Status: Survived}, {Status: NotCovered}}},
		{Name: "boolean", Mutations: []MutationResult{{Status: Error}, {Status: Timeout}, {Status: Skipped}}},
	}}

	assert.Equal(t, Summary{Total: 6, Killed: 1, Survived: 1, Skipped: 1, Timeout: 1, Error: 1, NotCovered: 1}, Summarize(r))
	assert.InDelta(t, 0.2, Summarize(r).Score(), 1e-9)
}

func TestSummary_Merge(t *testing.T) {
	a := Summary{Total: 3, Killed: 1, Error: 1, NotCovered: 1}
	a.Merge(Summary{Total: 4, Survived: 1, Skipped: 1, Timeout: 1, Error: 1})
	a.Merge(Summary{Total: 2, NotCovered: 2})

	assert.Equal(t, Summary{Total: 9, Killed: 1, Survived: 1, Skipped: 1, Timeout: 1, Error: 2, NotCovered: 3}, a)
}

func TestScore_NonPositiveScoredIsPerfect(t *testing.T) {
	assert.InDelta(t, 1.0, Score(0, 0), 1e-9)
	assert.InDelta(t, 1.0, Score(3, -1), 1e-9)
}

func TestScore_SingleMutation(t *testing.T) {
	assert.InDelta(t, 0.0, Score(0, 1), 1e-9)
	assert.InDelta(t, 1.0, Score(1, 1), 1e-9)
}

func TestSummary_TimeoutsKilled(t *testing.T) {
	s := Summary{Total: 4, Killed: 2, Timeout: 1, Survived: 1}
	assert.InDelta(t, 0.5, s.Score(), 0, "timeouts count as not killed by default")

	s.TimeoutsKilled = true
	assert.InDelta(t, 0.75, s.Score(), 0)

	var merged Summary
	merged.Merge(s)
	assert.True(t, merged.TimeoutsKilled, "a merge keeps the policy")
	assert.InDelta(t, 0.75, merged.Score(), 0)
}
