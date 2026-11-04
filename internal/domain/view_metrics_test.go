package domain_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gooze.dev/pkg/gooze/internal/adapter"
	domain "gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// The view fixture holds 4 mutants: 1 killed, 1 survived, 1 error and 1
// not_covered, so 1 of the 3 that count is killed.
const fixtureMetrics = `gooze_mutation_score_percent 33.33
gooze_mutants_total 4
gooze_mutants_killed 1
gooze_mutants_survived 1
gooze_mutants_not_covered 1
gooze_mutants_timeout 0
gooze_mutants_skipped 0
gooze_mutants_error 1
# EOF
`

func TestWorkflow_View_MetricsFormat(t *testing.T) {
	out, err := viewFormatted(t, domain.ViewArgs{Reports: m.Path(viewFixture(t)), Format: domain.FormatMetrics}, false)
	require.NoError(t, err)

	assert.Equal(t, fixtureMetrics, out)
}

func TestWorkflow_View_MetricsFormatNamesADiffRunsMetrics(t *testing.T) {
	dir := viewFixture(t)
	require.NoError(t, adapter.NewReportStore().SaveSelection(context.Background(), m.Path(dir), rpt.Selection{Diff: "origin/main", Base: "1a2b3c4d"}))

	out, err := viewFormatted(t, domain.ViewArgs{Reports: m.Path(dir), Format: domain.FormatMetrics}, false)
	require.NoError(t, err)

	assert.Equal(t, `gooze_diff_mutation_score_percent 33.33
gooze_diff_mutants_total 4
gooze_diff_mutants_killed 1
gooze_diff_mutants_survived 1
gooze_diff_mutants_not_covered 1
gooze_diff_mutants_timeout 0
gooze_diff_mutants_skipped 0
gooze_diff_mutants_error 1
# EOF
`, out, "a --diff run scores only the changed lines, so its metrics don't compare with a full run's")
}

func TestWorkflow_View_MetricsFormatWithoutMutations(t *testing.T) {
	out, err := viewFormatted(t, domain.ViewArgs{Reports: m.Path(t.TempDir()), Format: domain.FormatMetrics}, false)
	require.NoError(t, err)

	assert.Equal(t, `gooze_mutants_total 0
gooze_mutants_killed 0
gooze_mutants_survived 0
gooze_mutants_not_covered 0
gooze_mutants_timeout 0
gooze_mutants_skipped 0
gooze_mutants_error 0
# EOF
`, out, "no score without mutations: 100% would read like a result")
}

func TestWorkflow_View_MetricsFormatWritesBeforeTheGate(t *testing.T) {
	out, err := viewFormatted(t, domain.ViewArgs{
		Reports:    m.Path(viewFixture(t)),
		Format:     domain.FormatMetrics,
		Thresholds: domain.Thresholds{Key: "threshold", Total: 100},
	}, false)

	require.Error(t, err)
	assert.Equal(t, m.ExitBelowThreshold, exitCodeOf(err))
	assert.Equal(t, fixtureMetrics, out)
}

func TestWorkflow_View_MetricsFormatScoresOneMutant(t *testing.T) {
	dir := t.TempDir()
	source := rpt.Source{Origin: &rpt.File{ShortPath: "a.go", FullPath: "/p/a.go", Hash: "a"}}
	writeReports(t, dir, m.Report{Source: source, Result: m.Result{mgen.Comparison: {{MutationID: "a-1", Status: rpt.Killed}}}})

	out, err := viewFormatted(t, domain.ViewArgs{Reports: m.Path(dir), Format: domain.FormatMetrics}, false)
	require.NoError(t, err)

	assert.Contains(t, out, "gooze_mutation_score_percent 100\ngooze_mutants_total 1\n")
}
