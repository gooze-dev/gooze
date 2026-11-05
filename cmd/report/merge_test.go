package report

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
	m "gooze.dev/pkg/gooze/internal/model"
)

func TestReportMerge_PassesTheDiffThresholds(t *testing.T) {
	viper.Set(testOutputKey, "/tmp/reports")
	viper.Set("threshold.max_survivors", 2)
	defer viper.Reset()

	wf := domainmocks.NewMockWorkflow(t)
	wf.On("Merge", mock.Anything, domain.MergeArgs{View: domain.ViewArgs{Gate: domain.GateAuto},
		Reports:        m.Path("/tmp/reports"),
		Thresholds:     domain.Thresholds{Key: "threshold", Total: 85, MaxSurvivors: 2},
		DiffThresholds: domain.Thresholds{Key: "threshold.diff", Total: 100, MaxSurvivors: 2},
	}).Return(nil)

	deps := Deps{Workflow: wf, Publisher: domainmocks.NewMockReportPublisher(t), OutputKey: testOutputKey}

	require.NoError(t, execReport(t, deps, "merge", "--threshold-total", "85", "--threshold-diff-total", "100"))
	wf.AssertExpectations(t)
}

func mergeArgsFor(t *testing.T, args ...string) domain.MergeArgs {
	t.Helper()

	var got domain.MergeArgs

	wf := domainmocks.NewMockWorkflow(t)
	wf.On("Merge", mock.Anything, mock.Anything).Run(func(a mock.Arguments) { got = a.Get(1).(domain.MergeArgs) }).Return(nil)

	deps := Deps{Workflow: wf, Publisher: domainmocks.NewMockReportPublisher(t), OutputKey: testOutputKey}
	require.NoError(t, execReport(t, deps, append([]string{"merge"}, args...)...))

	return got
}

func TestReportMerge_Scope(t *testing.T) {
	assert.False(t, mergeArgsFor(t).Scopes, "the whole project only, by default")
	assert.False(t, mergeArgsFor(t, "--scope", "total").Scopes)
	assert.True(t, mergeArgsFor(t, "--scope", "diff,total").Scopes)
	assert.True(t, mergeArgsFor(t, "--scope", "total,diff").Scopes)

	cq := filepath.Join(t.TempDir(), "gl-code-quality.json")
	got := mergeArgsFor(t, "--scope", "diff,total", "--write-diff", "gitlab="+cq, "--write-diff", "text="+cq+".txt")
	require.Len(t, got.DiffWrites, 2)
	assert.Equal(t, domain.FormatGitLab, got.DiffWrites[0].Format)
}

func TestReportMerge_ScopeUsageErrors(t *testing.T) {
	for name, tt := range map[string]struct {
		args []string
		want string
	}{
		"unknown":          {args: []string{"--scope", "lines"}, want: `invalid --scope "lines": must be total or diff,total`},
		"diff alone":       {args: []string{"--scope", "diff"}, want: `invalid --scope "diff": must be total or diff,total; for the changed lines alone, merge -o <reports>/run`},
		"write-diff alone": {args: []string{"--write-diff", "gitlab=" + filepath.Join(os.TempDir(), "gooze-never-written.json")}, want: "--write-diff writes the changed lines' scope; add --scope diff,total"},
	} {
		t.Run(name, func(t *testing.T) {
			deps := Deps{Workflow: domainmocks.NewMockWorkflow(t), Publisher: domainmocks.NewMockReportPublisher(t), OutputKey: testOutputKey}
			err := execReport(t, deps, append([]string{"merge"}, tt.args...)...)

			require.EqualError(t, err, tt.want)
			assert.Equal(t, m.ExitUsage, codeOf(err))
		})
	}
}
