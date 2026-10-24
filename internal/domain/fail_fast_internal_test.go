package domain

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	m "gooze.dev/pkg/gooze/internal/model"
)

func TestNewFailFastWatch(t *testing.T) {
	all := Thresholds{Key: "threshold", Total: 80, Average: 70, File: 60, MaxSurvivors: 3}
	stop := func(error) {}

	cases := map[string]struct {
		policy  FailFast
		gates   Thresholds
		sharded bool
		want    *Thresholds // nil: no watch
	}{
		"off":                                    {FailFastOff, all, false, nil},
		"empty is off":                           {"", all, false, nil},
		"unknown is off":                         {"later", all, false, nil},
		"gate checks every gate":                 {FailFastGate, all, false, &all},
		"gate without gates":                     {FailFastGate, Thresholds{Key: "threshold"}, false, nil},
		"gate on a shard counts survivors":       {FailFastGate, all, true, &Thresholds{Key: "threshold", MaxSurvivors: 3}},
		"gate on a shard without a survivor cap": {FailFastGate, Thresholds{Key: "threshold", Total: 80}, true, nil},
		"file checks the file threshold only":    {FailFastFile, all, false, &Thresholds{Key: "threshold", File: 60}},
		"file without a file threshold":          {FailFastFile, Thresholds{Key: "threshold", Total: 80}, false, nil},
		"file on a shard":                        {FailFastFile, all, true, nil},
		"first needs no gates":                   {FailFastFirst, Thresholds{}, false, &Thresholds{}},
		"first on a shard":                       {FailFastFirst, all, true, &Thresholds{}},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			watch := newFailFastWatch(tc.policy, tc.gates, tc.sharded, stop)
			if tc.want == nil {
				assert.Nil(t, watch)
				return
			}

			require.NotNil(t, watch)
			assert.Equal(t, tc.policy, watch.policy)
			assert.Equal(t, *tc.want, watch.gates)
		})
	}
}

func TestFailFastWatch_NilNeverActs(t *testing.T) {
	var watch *failFastWatch

	require.NoError(t, watch.plan(nil, nil))
	assert.False(t, watch.skips("a.go"))
	assert.Empty(t, watch.actedOn())
}

func TestFailFastWatch_StopsOnce(t *testing.T) {
	ctx, stop := context.WithCancelCause(context.Background())
	defer stop(nil)

	stops := 0
	watch := newFailFastWatch(FailFastFirst, Thresholds{}, false, func(err error) { stops++; stop(err) })

	watch.halt(&FailFastError{Position: "a.go:1"})
	watch.halt(&FailFastError{Position: "a.go:2"})

	assert.Equal(t, 1, stops)
	assert.EqualError(t, context.Cause(ctx), "stopped at the first surviving mutant, a.go:1 (fail-fast first)")
	assert.Equal(t, FailFastFirst, watch.actedOn())
}

func TestUntestedErr_FailingTestsOutrankOnlyAGateVerdict(t *testing.T) {
	var untested untestedPackages
	untested.add(&FailingTestsError{Package: "./calc"})

	failing := untested.err()
	require.Error(t, failing)

	verdict := WithExitCode(m.ExitBelowThreshold, &FailFastError{Position: "a.go:1"})
	assert.Equal(t, failing, untestedErr(verdict, &untested), "a gate's verdict leaves out the untested packages")

	for _, code := range []m.ExitCode{m.ExitUsage, m.ExitGit} {
		other := WithExitCode(code, errors.New("other"))
		assert.Equal(t, other, untestedErr(other, &untested), "exit code %d outranks failing tests", code)
	}

	assert.Equal(t, verdict, untestedErr(verdict, &untestedPackages{}))
	assert.NoError(t, untestedErr(nil, &untestedPackages{}))
}
