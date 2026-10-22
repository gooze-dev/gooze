package domain

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"

	m "gooze.dev/pkg/gooze/internal/model"
)

func TestInterruptedOnly(t *testing.T) {
	for _, tc := range []struct {
		scope *rpt.Scope
		want  bool
	}{
		{scope: nil, want: false},
		{scope: &rpt.Scope{}, want: false},
		{scope: &rpt.Scope{Interrupted: true}, want: true},
		{scope: &rpt.Scope{Interrupted: true, Base: "main"}, want: false},
		{scope: &rpt.Scope{Interrupted: true, Lines: []rpt.LineRange{{Start: 1, End: 2}}}, want: false},
		{scope: &rpt.Scope{Interrupted: true, Run: "^Add$"}, want: false},
		{scope: &rpt.Scope{Interrupted: true, Mutagens: []string{"arithmetic"}}, want: false},
	} {
		assert.Equal(t, tc.want, interruptedOnly(tc.scope), "%+v", tc.scope)
	}
}

func TestOfTypes(t *testing.T) {
	report := m.Report{Result: m.Result{mgen.Arithmetic: nil}}
	old := mgen.Type{Name: mgen.Arithmetic.Name, Version: mgen.Arithmetic.Version - 1}

	assert.True(t, ofTypes(report, []mgen.Type{mgen.Boolean, mgen.Arithmetic}))
	assert.False(t, ofTypes(report, []mgen.Type{old}), "a result of another mutagen version is not resumed")
	assert.False(t, ofTypes(report, nil))
}

func TestRunError(t *testing.T) {
	failed := errors.New("dispatch failed")
	fatal := errors.New("spill full")
	mutant := errors.New("workspace broken")

	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	assert.ErrorIs(t, runError(context.Background(), failed, collectorResult{}), failed)
	assert.ErrorIs(t, runError(context.Background(), nil, collectorResult{fatalErr: fatal}), fatal)
	assert.NoError(t, runError(context.Background(), nil, collectorResult{}))
	assert.NoError(t, runError(canceled, context.Canceled, collectorResult{}), "an interruption keeps the finished results")
	assert.ErrorIs(t, runError(canceled, nil, collectorResult{fatalErr: fatal}), fatal)
	assert.ErrorIs(t, runError(canceled, nil, collectorResult{errs: []error{mutant}}), mutant)
}
