package report

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
	m "gooze.dev/pkg/gooze/internal/model"
	"gooze.dev/pkg/gooze/pkg/config"
)

func TestReportView_MaxPerFile(t *testing.T) {
	defer viper.Reset()

	assert.Zero(t, viewArgsFor(t).MaxPerFile, "every line is listed by default")
	assert.Equal(t, 7, viewArgsFor(t, "--max-per-file", "7").MaxPerFile)

	viper.Set(config.KeyReportMaxPerFile, " 5 ")
	assert.Equal(t, 5, viewArgsFor(t).MaxPerFile, "report.max_per_file sets the default")
	assert.Equal(t, 2, viewArgsFor(t, "--max-per-file", "2").MaxPerFile, "the flag wins")
	assert.Zero(t, viewArgsFor(t, "--max-per-file", "0").MaxPerFile, "0 lifts the configured cap")

	viper.Set(config.KeyReportMaxPerFile, "010")
	assert.Equal(t, 10, viewArgsFor(t).MaxPerFile, "base 10")
}

func TestReportView_MaxPerFileUsageErrors(t *testing.T) {
	for name, tt := range map[string]struct {
		args   []string
		config string
		want   string
	}{
		"negative flag":   {args: []string{"--max-per-file", "-1"}, want: "invalid --max-per-file: -1 is out of range: must be at least 0"},
		"negative config": {config: "-2", want: "invalid report.max_per_file: -2 is out of range: must be at least 0"},
		"not a number":    {config: "seven", want: `invalid report.max_per_file: "seven" is not a whole number`},
	} {
		t.Run(name, func(t *testing.T) {
			defer viper.Reset()

			if tt.config != "" {
				viper.Set(config.KeyReportMaxPerFile, tt.config)
			}

			deps := Deps{Workflow: domainmocks.NewMockWorkflow(t), Publisher: domainmocks.NewMockReportPublisher(t), OutputKey: testOutputKey}
			err := execReport(t, deps, append([]string{"view"}, tt.args...)...)

			require.EqualError(t, err, tt.want)
			assert.Equal(t, m.ExitUsage, codeOf(err))
		})
	}
}

func TestReportViewAndMerge_MaxPerFileFlag(t *testing.T) {
	for _, cmd := range []*cobra.Command{newViewCmd(Deps{}), newMergeCmd(Deps{})} {
		flag := cmd.Flags().Lookup("max-per-file")
		require.NotNil(t, flag, cmd.Name())
		assert.Equal(t, "0", flag.DefValue, "the help shows the cap as off")
	}
}
