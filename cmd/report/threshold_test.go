package report

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
	m "gooze.dev/pkg/gooze/internal/model"
	"gooze.dev/pkg/gooze/pkg/config"
)

// readThresholds parses args for a command with threshold flags under key and
// flagPrefix, and returns what the flags read.
func readThresholds(t *testing.T, key, flagPrefix string, args ...string) (domain.Thresholds, error) {
	t.Helper()

	cmd := &cobra.Command{Use: "x", RunE: func(*cobra.Command, []string) error { return nil }}
	flags := AddThresholdFlags(cmd, key, flagPrefix)
	require.NoError(t, cmd.ParseFlags(args))

	return flags.Read(cmd)
}

func TestThresholdFlags(t *testing.T) {
	defer viper.Reset()

	got, err := readThresholds(t, "threshold", "threshold")
	require.NoError(t, err)
	assert.Equal(t, domain.Thresholds{Key: "threshold"}, got, "off by default")

	viper.Set(config.KeyThresholdTotal, 80)
	viper.Set(config.KeyThresholdFile, "60.5")
	viper.Set(config.KeyThresholdDiffAverage, 90)

	got, err = readThresholds(t, "threshold", "threshold", "--threshold-average", "70.25")
	require.NoError(t, err)
	assert.Equal(t, domain.Thresholds{Key: "threshold", Total: 80, Average: 70.25, File: 60.5}, got)

	got, err = readThresholds(t, "threshold", "threshold", "--threshold-total", "0")
	require.NoError(t, err)
	assert.Equal(t, domain.Thresholds{Key: "threshold", File: 60.5}, got, "a flag of 0 turns the config value off")

	got, err = readThresholds(t, "threshold.diff", "threshold-diff", "--threshold-diff-file", "95")
	require.NoError(t, err)
	assert.Equal(t, domain.Thresholds{Key: "threshold.diff", Average: 90, File: 95}, got)
}

func TestThresholdFlags_OutOfRange(t *testing.T) {
	defer viper.Reset()

	_, err := readThresholds(t, "threshold", "threshold", "--threshold-total", "101")
	require.EqualError(t, err, "invalid --threshold-total: 101 is out of range: must be between 0 and 100")
	assert.Equal(t, m.ExitUsage, codeOf(err))

	viper.Set(config.KeyThresholdDiffFile, -2)
	_, err = readThresholds(t, "threshold.diff", "threshold-diff")
	require.EqualError(t, err, "invalid threshold.diff.file: -2 is out of range: must be between 0 and 100")
	assert.Equal(t, m.ExitUsage, codeOf(err))

	viper.Set(config.KeyThresholdDiffFile, "lots")
	_, err = readThresholds(t, "threshold.diff", "threshold-diff")
	require.EqualError(t, err, `invalid threshold.diff.file: "lots" is not a number`)
}

func TestReportViewAndMergePassThresholds(t *testing.T) {
	viper.Set(testOutputKey, "/tmp/reports")
	viper.Set(config.KeyThresholdAverage, 50)
	defer viper.Reset()

	want := domain.Thresholds{Key: "threshold", Total: 75, Average: 50}

	wf := domainmocks.NewMockWorkflow(t)
	wf.On("View", mock.Anything, domain.ViewArgs{Reports: "/tmp/reports", Paths: []m.Path{}, Thresholds: want}).Return(nil)
	wf.On("Merge", mock.Anything, domain.MergeArgs{Reports: "/tmp/reports", Thresholds: want}).Return(nil)

	deps := Deps{Workflow: wf, Publisher: domainmocks.NewMockReportPublisher(t), OutputKey: testOutputKey}

	require.NoError(t, execReport(t, deps, "view", "--threshold-total", "75"))
	require.NoError(t, execReport(t, deps, "merge", "--threshold-total", "75"))

	err := execReport(t, deps, "merge", "--threshold-file", "100.5")
	assert.Equal(t, m.ExitUsage, codeOf(err))
	wf.AssertNumberOfCalls(t, "Merge", 1)
}

// codeOf returns the exit code err carries, or ExitFailure when none.
func codeOf(err error) m.ExitCode {
	var coded *m.CodedError
	if errors.As(err, &coded) {
		return coded.Code
	}

	return m.ExitFailure
}
