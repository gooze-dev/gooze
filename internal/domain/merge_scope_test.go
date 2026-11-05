package domain_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"gooze.dev/pkg/gooze/internal/adapter"
	domain "gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
	m "gooze.dev/pkg/gooze/internal/model"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

func otherSource() rpt.Source {
	return rpt.Source{Origin: &rpt.File{FullPath: "/p/other.go", ShortPath: "other.go", Hash: "o1"}}
}

// scopeFixture writes a baseline (calc.go: a killed, b survived; other.go: c
// killed) and two --diff shards in run/ that re-test the changed calc.go
// (x killed, y survived), and returns the base dir.
func scopeFixture(t *testing.T) string {
	t.Helper()

	base := t.TempDir()
	saveReports(t, base,
		mergeReport(mergeSource("h1"), "a", rpt.Killed, nil),
		mergeReport(mergeSource("h1"), "b", rpt.Survived, nil),
		mergeReport(otherSource(), "c", rpt.Killed, nil),
	)

	for i, status := range []rpt.TestStatus{rpt.Killed, rpt.Survived} {
		shard := filepath.Join(base, domain.RunDirName, domain.ShardDirPrefix+string(rune('0'+i)))
		saveReports(t, shard, mergeReport(mergeSource("h2"), []string{"x", "y"}[i], status, nil))
		saveSelection(t, shard, rpt.Selection{Diff: "main"})
	}

	return base
}

// mergeScopes merges base with both scopes and returns the summaries it
// showed, in order, and the error.
func mergeScopes(t *testing.T, base string, args domain.MergeArgs) ([]domain.ReportSummary, error) {
	t.Helper()

	var shown []domain.ReportSummary

	reporter := domainmocks.NewMockReporter(t)
	reporter.EXPECT().DisplayReportSummary(mock.Anything, mock.Anything).
		Run(func(_ context.Context, s domain.ReportSummary) { shown = append(shown, s) }).Maybe()

	wf := domain.NewWorkflow(nil, adapter.NewReportStore(), reporter, nil, nil)
	args.Reports, args.Scopes = m.Path(base), true

	return shown, wf.Merge(context.Background(), args)
}

func TestMerge_ScopesShowTheDiffThenTheWholeProject(t *testing.T) {
	base := scopeFixture(t)

	var diffCQ, metrics bytes.Buffer

	shown, err := mergeScopes(t, base, domain.MergeArgs{
		View:       domain.ViewArgs{Writes: []domain.ReportWrite{{Format: domain.FormatMetrics, Output: &metrics}}},
		DiffWrites: []domain.ReportWrite{{Format: domain.FormatGitLab, Output: &diffCQ}},
	})
	require.NoError(t, err)
	require.Len(t, shown, 2)

	assert.Equal(t, rpt.Summary{Total: 2, Killed: 1, Survived: 1}, shown[0].Score.Summary, "the changed lines")
	assert.Equal(t, "changed lines since main", shown[0].Score.Selection)
	assert.Equal(t, rpt.Summary{Total: 3, Killed: 2, Survived: 1}, shown[1].Score.Summary, "the baseline with the run folded in")
	assert.Empty(t, shown[1].Score.Selection)

	assert.Contains(t, diffCQ.String(), `"check_name": "gooze/survived"`)
	assert.Equal(t, "gooze_mutation_score_percent 66.67\n"+
		"gooze_mutants_total 3\ngooze_mutants_killed 2\ngooze_mutants_survived 1\ngooze_mutants_not_covered 0\n"+
		"gooze_mutants_timeout 0\ngooze_mutants_skipped 0\ngooze_mutants_error 0\n"+
		"gooze_diff_mutation_score_percent 50\n"+
		"gooze_diff_mutants_total 2\ngooze_diff_mutants_killed 1\ngooze_diff_mutants_survived 1\ngooze_diff_mutants_not_covered 0\n"+
		"gooze_diff_mutants_timeout 0\ngooze_diff_mutants_skipped 0\ngooze_diff_mutants_error 0\n"+
		"# EOF\n", metrics.String(), "one file, both scopes, one # EOF")
	assert.Equal(t, 1, strings.Count(metrics.String(), "# EOF"))

	// run/ stays merged and viewable.
	entries, err := os.ReadDir(filepath.Join(base, domain.RunDirName))
	require.NoError(t, err)

	for _, entry := range entries {
		assert.False(t, strings.HasPrefix(entry.Name(), domain.ShardDirPrefix), "the shards are merged: %s", entry.Name())
	}

	viewed := viewSummary(t, domain.ViewArgs{Reports: m.Path(filepath.Join(base, domain.RunDirName))})
	assert.Equal(t, rpt.Summary{Total: 2, Killed: 1, Survived: 1}, viewed.Score.Summary)
}

func TestMerge_ScopesGate(t *testing.T) {
	diffLow := domain.Thresholds{Key: "threshold.diff", Total: 60}
	totalLow := domain.Thresholds{Key: "threshold", Total: 70}

	for name, tt := range map[string]struct {
		gate domain.Gate
		want string
	}{
		"auto is diff": {gate: domain.GateAuto, want: "mutation score 50.00% is below threshold.diff.total 60%"},
		"diff":         {gate: domain.GateDiff, want: "mutation score 50.00% is below threshold.diff.total 60%"},
		"total":        {gate: domain.GateTotal, want: "mutation score 66.67% is below threshold.total 70%"},
		"none":         {gate: domain.GateNone},
	} {
		t.Run(name, func(t *testing.T) {
			shown, err := mergeScopes(t, scopeFixture(t), domain.MergeArgs{
				View: domain.ViewArgs{Gate: tt.gate}, Thresholds: totalLow, DiffThresholds: diffLow,
			})

			assert.Len(t, shown, 2, "both scopes are shown, also when the gate fails")

			if tt.want == "" {
				require.NoError(t, err)
				return
			}

			require.EqualError(t, err, tt.want, "one error, naming its scope by its key")
			assert.Equal(t, m.ExitBelowThreshold, exitCodeOf(err))
		})
	}
}

func TestMerge_ScopesNeedDiffReports(t *testing.T) {
	base := t.TempDir()
	saveReports(t, base, mergeReport(mergeSource("h1"), "a", rpt.Killed, nil))

	_, err := mergeScopes(t, base, domain.MergeArgs{})
	require.EqualError(t, err, "--scope diff needs the reports of a gooze run --diff in "+filepath.Join(base, domain.RunDirName))
	assert.Equal(t, m.ExitNoReports, exitCodeOf(err))
}

func TestMerge_ScopesStopOnShardsOfDifferentRuns(t *testing.T) {
	base := t.TempDir()
	saveReports(t, base, mergeReport(mergeSource("h1"), "a", rpt.Killed, nil))

	for i, ref := range []string{"main", "develop"} {
		shard := filepath.Join(base, domain.RunDirName, domain.ShardDirPrefix+string(rune('0'+i)))
		saveReports(t, shard, mergeReport(mergeSource("h2"), ref, rpt.Killed, nil))
		saveSelection(t, shard, rpt.Selection{Diff: ref})
	}

	shown, err := mergeScopes(t, base, domain.MergeArgs{})
	require.ErrorContains(t, err, "shards were run with different selections")
	assert.Empty(t, shown)
}
