package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"gooze.dev/pkg/gooze/cmd/report"
	"gooze.dev/pkg/gooze/internal/adapter"
	"gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
	m "gooze.dev/pkg/gooze/internal/model"
	"gooze.dev/pkg/gooze/pkg"
	"gooze.dev/pkg/gooze/pkg/config"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// halfKilledReports writes reports scoring 50% (1 killed, 1 survived) to a
// new directory and, when diff is set, records them as a --diff run's.
func halfKilledReports(t *testing.T, diff bool) string {
	t.Helper()

	ctx := context.Background()
	dir := t.TempDir()
	store := adapter.NewReportStore()

	spill, err := pkg.NewFileSpill[m.Report]()
	require.NoError(t, err)
	t.Cleanup(func() { _ = spill.Close() })

	require.NoError(t, spill.Append(m.Report{
		Source: rpt.Source{Origin: &rpt.File{ShortPath: "calc.go", FullPath: "/p/calc.go", Hash: "c"}},
		Result: m.Result{mgen.Comparison: {{MutationID: "k", Status: rpt.Killed}, {MutationID: "s", Status: rpt.Survived}}},
	}))
	require.NoError(t, store.SaveReports(ctx, m.Path(dir), spill))

	if diff {
		require.NoError(t, store.SaveSelection(ctx, m.Path(dir), rpt.Selection{Diff: "origin/main", Base: "1a2b3c4d"}))
	}

	return dir
}

// viewReports runs gooze report view on dir with args, reading the
// thresholds from flags, gooze.yaml (yaml) and GOOZE_* env vars.
func viewReports(t *testing.T, dir, yaml string, args ...string) error {
	t.Helper()

	require.NoError(t, viper.ReadConfig(strings.NewReader(yaml)))
	t.Cleanup(func() { require.NoError(t, viper.ReadConfig(strings.NewReader(""))) })

	viper.Set(config.KeyOutput, dir)
	t.Cleanup(func() { viper.Set(config.KeyOutput, ".gooze-reports") })

	reporter := domainmocks.NewMockReporter(t)
	reporter.EXPECT().DisplayReportSummary(mock.Anything, mock.Anything).Return().Maybe()

	cmd := report.New(report.Deps{
		Workflow:  domain.NewWorkflow(nil, adapter.NewReportStore(), reporter, nil, nil),
		OutputKey: config.KeyOutput,
	})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(append([]string{"view"}, args...))

	return cmd.Execute()
}

func TestReportView_DiffThresholdsOnADiffRun(t *testing.T) {
	const config100 = "threshold:\n  diff:\n    total: 100\n"

	tests := []struct {
		name string
		yaml string
		env  string
		args []string
		diff bool
		want string // "" passes, otherwise the error
	}{
		{name: "flag only", args: []string{"--threshold-diff-total", "60"}, diff: true,
			want: "mutation score 50.00% is below threshold.diff.total 60%"},
		{name: "config only", yaml: config100, diff: true,
			want: "mutation score 50.00% is below threshold.diff.total 100%"},
		{name: "env only", env: "100", diff: true,
			want: "mutation score 50.00% is below threshold.diff.total 100%"},
		{name: "flag overrides config", yaml: config100, args: []string{"--threshold-diff-total", "50"}, diff: true},
		{name: "flag 0 turns config off", yaml: config100, args: []string{"--threshold-diff-total", "0"}, diff: true},
		{name: "main reports use the plain thresholds", yaml: config100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env != "" {
				t.Setenv("GOOZE_THRESHOLD_DIFF_TOTAL", tt.env)
			}

			err := viewReports(t, halfKilledReports(t, tt.diff), tt.yaml, tt.args...)
			if tt.want == "" {
				require.NoError(t, err)
				return
			}

			require.EqualError(t, err, tt.want)
			assert.Equal(t, m.ExitBelowThreshold, exitCode(context.Background(), err, false))
		})
	}
}
