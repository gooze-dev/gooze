package domain

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	m "gooze.dev/pkg/gooze/internal/model"
	goozepkg "gooze.dev/pkg/gooze/pkg"
)

type errSpill[T any] struct {
	err error
}

func (e errSpill[T]) Len() uint64                                    { return 0 }
func (e errSpill[T]) Path() string                                   { return "" }
func (e errSpill[T]) Append(_ T) error                               { return nil }
func (e errSpill[T]) AppendBatch(_ []T) error                        { return nil }
func (e errSpill[T]) Get(_ uint64) (T, error)                        { var zero T; return zero, errors.New("not implemented") }
func (e errSpill[T]) Range(_ func(index uint64, item T) error) error { return e.err }
func (e errSpill[T]) Close() error                                   { return nil }

func TestMutationScoreFromReports(t *testing.T) {
	spill, err := goozepkg.NewFileSpill[m.Report]()
	require.NoError(t, err)
	defer spill.Close()

	report := m.Report{
		Result: m.Result{
			m.MutationBoolean: {
				{MutationID: "m1", Status: m.Killed, Err: nil},
				{MutationID: "m2", Status: m.Survived, Err: nil},
				{MutationID: "m3", Status: m.Skipped, Err: nil},
				{MutationID: "m4", Status: m.Error, Err: nil},
				{MutationID: "m5", Status: m.Killed, Err: nil},
			},
		},
	}

	err = spill.Append(report)
	require.NoError(t, err)

	score, err := mutationScoreFromReports(spill)
	require.NoError(t, err)

	require.Equal(t, 0.4, score)
}

func TestMutationScoreFromReports_EmptySpillIs100(t *testing.T) {
	spill, err := goozepkg.NewFileSpill[m.Report]()
	require.NoError(t, err)
	defer spill.Close()

	score, err := mutationScoreFromReports(spill)
	require.NoError(t, err)

	require.Equal(t, 100.0, score)
}

func TestMutationScoreFromReports_OnlySkippedAndErrorIs100(t *testing.T) {
	spill, err := goozepkg.NewFileSpill[m.Report]()
	require.NoError(t, err)
	defer spill.Close()

	report := m.Report{
		Result: m.Result{
			m.MutationBoolean: {
				{MutationID: "m1", Status: m.Skipped, Err: nil},
				{MutationID: "m2", Status: m.Error, Err: nil},
			},
		},
	}
	err = spill.Append(report)
	require.NoError(t, err)

	score, err := mutationScoreFromReports(spill)
	require.NoError(t, err)

	require.Equal(t, 0.0, score)
}

func TestMutationScoreFromReports_AllSurvivedIs0(t *testing.T) {
	spill, err := goozepkg.NewFileSpill[m.Report]()
	require.NoError(t, err)
	defer spill.Close()

	report := m.Report{
		Result: m.Result{
			m.MutationBoolean: {
				{MutationID: "m1", Status: m.Survived, Err: nil},
				{MutationID: "m2", Status: m.Survived, Err: nil},
			},
		},
	}
	err = spill.Append(report)
	require.NoError(t, err)

	score, err := mutationScoreFromReports(spill)
	require.NoError(t, err)

	require.Equal(t, 0.0, score)
}

func TestMutationScoreFromReports_AggregatesAcrossReports(t *testing.T) {
	spill, err := goozepkg.NewFileSpill[m.Report]()
	require.NoError(t, err)
	defer spill.Close()

	report1 := m.Report{
		Result: m.Result{
			m.MutationArithmetic: {
				{MutationID: "a1", Status: m.Killed, Err: nil},
				{MutationID: "a2", Status: m.Survived, Err: nil},
			},
		},
	}
	report2 := m.Report{
		Result: m.Result{
			m.MutationBoolean: {
				{MutationID: "b1", Status: m.Killed, Err: nil},
				{MutationID: "b2", Status: m.Killed, Err: nil},
				{MutationID: "b3", Status: m.Killed, Err: nil},
				{MutationID: "b4", Status: m.Error, Err: nil},
				{MutationID: "b5", Status: m.Survived, Err: nil},
				{MutationID: "b6", Status: m.Timeout, Err: nil},
			},
		},
	}

	err = spill.Append(report1)
	require.NoError(t, err)
	err = spill.Append(report2)
	require.NoError(t, err)

	score, err := mutationScoreFromReports(spill)
	require.NoError(t, err)

	require.Equal(t, 0.5, score)
}

func TestMutationScoreFromReports_RangeErrorPropagates(t *testing.T) {
	wantErr := errors.New("range failed")

	_, err := mutationScoreFromReports(errSpill[m.Report]{err: wantErr})
	require.Error(t, err)
	require.ErrorIs(t, err, wantErr)
}
