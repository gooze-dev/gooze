package domain

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	m "gooze.dev/pkg/gooze/internal/model"
	goozepkg "gooze.dev/pkg/gooze/pkg"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
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

// scoreOf is the score of every report in the spill.
func scoreOf(reports goozepkg.FileSpill[m.Report]) (float64, error) {
	summary, err := summarizeReports(reports, nil)

	return summary.Score(), err
}

func TestSummarizeReports(t *testing.T) {
	spill, err := goozepkg.NewFileSpill[m.Report]()
	require.NoError(t, err)
	defer spill.Close()

	report := m.Report{
		Result: m.Result{
			mgen.Boolean: {
				{MutationID: "m1", Status: rpt.Killed, Err: nil},
				{MutationID: "m2", Status: rpt.Survived, Err: nil},
				{MutationID: "m3", Status: rpt.Skipped, Err: nil},
				{MutationID: "m4", Status: rpt.Error, Err: nil},
				{MutationID: "m5", Status: rpt.Killed, Err: nil},
			},
		},
	}

	err = spill.Append(report)
	require.NoError(t, err)

	score, err := scoreOf(spill)
	require.NoError(t, err)

	require.Equal(t, 0.5, score)
}

func TestSummarizeReports_NotCoveredCountsAsSurvivor(t *testing.T) {
	spill, err := goozepkg.NewFileSpill[m.Report]()
	require.NoError(t, err)
	defer spill.Close()

	report := m.Report{
		Result: m.Result{
			mgen.Boolean: {
				{MutationID: "m1", Status: rpt.Killed, Err: nil},
				{MutationID: "m2", Status: rpt.NotCovered, Err: nil},
			},
		},
	}

	require.NoError(t, spill.Append(report))

	score, err := scoreOf(spill)
	require.NoError(t, err)

	require.Equal(t, 0.5, score)
}

func TestSummarizeReports_EmptySpillIsFull(t *testing.T) {
	spill, err := goozepkg.NewFileSpill[m.Report]()
	require.NoError(t, err)
	defer spill.Close()

	score, err := scoreOf(spill)
	require.NoError(t, err)

	require.Equal(t, 1.0, score)
}

func TestSummarizeReports_OnlySkippedAndErrorIs100(t *testing.T) {
	spill, err := goozepkg.NewFileSpill[m.Report]()
	require.NoError(t, err)
	defer spill.Close()

	report := m.Report{
		Result: m.Result{
			mgen.Boolean: {
				{MutationID: "m1", Status: rpt.Skipped, Err: nil},
				{MutationID: "m2", Status: rpt.Error, Err: nil},
			},
		},
	}
	err = spill.Append(report)
	require.NoError(t, err)

	score, err := scoreOf(spill)
	require.NoError(t, err)

	require.Equal(t, 0.0, score)
}

func TestSummarizeReports_AllSurvivedIs0(t *testing.T) {
	spill, err := goozepkg.NewFileSpill[m.Report]()
	require.NoError(t, err)
	defer spill.Close()

	report := m.Report{
		Result: m.Result{
			mgen.Boolean: {
				{MutationID: "m1", Status: rpt.Survived, Err: nil},
				{MutationID: "m2", Status: rpt.Survived, Err: nil},
			},
		},
	}
	err = spill.Append(report)
	require.NoError(t, err)

	score, err := scoreOf(spill)
	require.NoError(t, err)

	require.Equal(t, 0.0, score)
}

func TestSummarizeReports_AggregatesAcrossReports(t *testing.T) {
	spill, err := goozepkg.NewFileSpill[m.Report]()
	require.NoError(t, err)
	defer spill.Close()

	report1 := m.Report{
		Result: m.Result{
			mgen.Arithmetic: {
				{MutationID: "a1", Status: rpt.Killed, Err: nil},
				{MutationID: "a2", Status: rpt.Survived, Err: nil},
			},
		},
	}
	report2 := m.Report{
		Result: m.Result{
			mgen.Boolean: {
				{MutationID: "b1", Status: rpt.Killed, Err: nil},
				{MutationID: "b2", Status: rpt.Killed, Err: nil},
				{MutationID: "b3", Status: rpt.Killed, Err: nil},
				{MutationID: "b4", Status: rpt.Error, Err: nil},
				{MutationID: "b5", Status: rpt.Survived, Err: nil},
				{MutationID: "b6", Status: rpt.Timeout, Err: nil},
			},
		},
	}

	err = spill.Append(report1)
	require.NoError(t, err)
	err = spill.Append(report2)
	require.NoError(t, err)

	score, err := scoreOf(spill)
	require.NoError(t, err)

	require.InDelta(t, 4.0/7.0, score, 1e-9)
}

func TestSummarizeReports_RangeErrorPropagates(t *testing.T) {
	wantErr := errors.New("range failed")

	_, err := scoreOf(errSpill[m.Report]{err: wantErr})
	require.Error(t, err)
	require.ErrorIs(t, err, wantErr)
}

func TestSummarizeReports_IncludeFiltersBySource(t *testing.T) {
	inScope := rpt.Source{Origin: &rpt.File{ShortPath: "a.go"}}
	outOfScope := rpt.Source{Origin: &rpt.File{ShortPath: "b.go"}}

	spill, err := goozepkg.FromSlice([]m.Report{
		{Source: inScope, Result: m.Result{mgen.Boolean: {{MutationID: "a1", Status: rpt.Killed}, {MutationID: "a2", Status: rpt.Survived}}}},
		{Source: outOfScope, Result: m.Result{mgen.Boolean: {{MutationID: "b1", Status: rpt.Survived}}}},
	})
	require.NoError(t, err)

	defer spill.Close()

	summary, err := summarizeReports(spill, func(source rpt.Source) bool { return source.Key() == "a.go" })
	require.NoError(t, err)
	require.Equal(t, rpt.Summary{Total: 2, Killed: 1, Survived: 1}, summary)
}
