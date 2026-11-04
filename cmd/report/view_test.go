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
