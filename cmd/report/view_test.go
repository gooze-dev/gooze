package report

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
	m "gooze.dev/pkg/gooze/internal/model"
	"gooze.dev/pkg/gooze/pkg/config"
)

func TestReportView_ByLine(t *testing.T) {
	assert.True(t, viewArgsFor(t, "--by-line").ByLine)
	assert.False(t, viewArgsFor(t).ByLine, "the flat list stays the default")
}

func TestReportView_ByLineGroupsTheGitHubFormatsSummary(t *testing.T) {
	stubShowPrefix(t, "", nil)

	args := viewArgsFor(t, "--by-line", "--format", "github")

	assert.True(t, args.ByLine)
	assert.Equal(t, domain.FormatGitHub, args.Format)
}

func TestReportView_ByLineUsageErrors(t *testing.T) {
	for name, tt := range map[string]struct {
		args   []string
		config string
		want   string
	}{
		"all":                 {args: []string{"--all"}, want: "--by-line groups the summary's survivors; --all lists every mutation"},
		"sarif":               {args: []string{"--format", "sarif"}, want: "--by-line groups the text summary; --format sarif writes none"},
		"codeclimate":         {args: []string{"--format", "codeclimate"}, want: "--by-line groups the text summary; --format codeclimate writes none"},
		"gitlab":              {args: []string{"--format", "gitlab"}, want: "--by-line groups the text summary; --format gitlab writes none"},
		"metrics":             {args: []string{"--format", "metrics"}, want: "--by-line groups the text summary; --format metrics writes none"},
		"configured format":   {config: "sarif", want: "--by-line groups the text summary; --format sarif writes none"},
		"all over the format": {args: []string{"--all"}, config: "sarif", want: "--by-line groups the summary's survivors; --all lists every mutation"},
	} {
		t.Run(name, func(t *testing.T) {
			defer viper.Reset()

			stubShowPrefix(t, "", nil)

			if tt.config != "" {
				viper.Set(config.KeyReportFormat, tt.config)
			}

			deps := Deps{Workflow: domainmocks.NewMockWorkflow(t), Publisher: domainmocks.NewMockReportPublisher(t), OutputKey: testOutputKey}
			err := execReport(t, deps, append([]string{"view", "--by-line"}, tt.args...)...)

			require.EqualError(t, err, tt.want)
			assert.Equal(t, m.ExitUsage, codeOf(err))
		})
	}
}

func TestReportView_SummaryTopAndSections(t *testing.T) {
	t.Setenv("GITLAB_CI", "")

	args := viewArgsFor(t, "--summary", "--top", "5")
	assert.True(t, args.Summary)
	assert.Equal(t, 5, args.Top)
	assert.False(t, args.Sections)

	plain := viewArgsFor(t)
	assert.False(t, plain.Summary)
	assert.Zero(t, plain.Top)

	t.Setenv("GITLAB_CI", "true")
	assert.True(t, viewArgsFor(t).Sections, "GitLab CI folds the long sections")
}

func TestReportView_SummaryUsageErrors(t *testing.T) {
	for name, tt := range map[string]struct {
		args []string
		want string
	}{
		"all":      {args: []string{"--summary", "--all"}, want: "--summary shortens the per-file summary; --all lists every mutation"},
		"by-line":  {args: []string{"--summary", "--by-line"}, want: "--summary lists no survivors to group by line"},
		"sarif":    {args: []string{"--summary", "--format", "sarif"}, want: "--summary shortens the text summary; --format sarif writes none"},
		"top":      {args: []string{"--top", "-1"}, want: "--top must be 0 (every file) or more, got -1"},
		"top, all": {args: []string{"--top", "3", "--all"}, want: "--top limits the per-file summary; --all lists every mutation"},
	} {
		t.Run(name, func(t *testing.T) {
			stubShowPrefix(t, "", nil)

			deps := Deps{Workflow: domainmocks.NewMockWorkflow(t), Publisher: domainmocks.NewMockReportPublisher(t), OutputKey: testOutputKey}
			err := execReport(t, deps, append([]string{"view"}, tt.args...)...)

			require.EqualError(t, err, tt.want)
			assert.Equal(t, m.ExitUsage, codeOf(err))
		})
	}
}
