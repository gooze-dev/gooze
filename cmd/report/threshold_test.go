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

func TestThresholdFlags_HelpShowsOff(t *testing.T) {
	cmd := &cobra.Command{Use: "x"}
	AddThresholdFlags(cmd, "threshold", "threshold")

	for _, name := range []string{"threshold-total", "threshold-average", "threshold-file"} {
		flag := cmd.Flags().Lookup(name)
		require.NotNil(t, flag, name)
		assert.Equal(t, "0", flag.DefValue, name)
	}
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

// readMaxSurvivors parses args for a command with the plain threshold flags
// and returns the max survivors they read.
func readMaxSurvivors(t *testing.T, args ...string) (int, error) {
	t.Helper()

	got, err := readThresholds(t, "threshold", "threshold", args...)

	return got.MaxSurvivors, err
}

func TestMaxSurvivorsFlag(t *testing.T) {
	defer viper.Reset()

	got, err := readMaxSurvivors(t)
	require.NoError(t, err)
	assert.Zero(t, got, "off by default")

	viper.Set(config.KeyThresholdMaxSurvivors, 5)

	got, err = readMaxSurvivors(t)
	require.NoError(t, err)
	assert.Equal(t, 5, got)

	got, err = readMaxSurvivors(t, "--threshold-max-survivors", "2")
	require.NoError(t, err)
	assert.Equal(t, 2, got, "the flag wins over the config")

	got, err = readMaxSurvivors(t, "--threshold-max-survivors", "0")
	require.NoError(t, err)
	assert.Zero(t, got, "a flag of 0 turns the config value off")

	// Env vars and quoted YAML are strings, read in base 10.
	viper.Set(config.KeyThresholdMaxSurvivors, "010")

	got, err = readMaxSurvivors(t)
	require.NoError(t, err)
	assert.Equal(t, 10, got)
}

func TestMaxSurvivorsFlag_HelpShowsOff(t *testing.T) {
	cmd := &cobra.Command{Use: "x"}
	AddThresholdFlags(cmd, "threshold", "threshold")

	flag := cmd.Flags().Lookup("threshold-max-survivors")
	require.NotNil(t, flag)
	assert.Equal(t, "0", flag.DefValue)
	assert.Equal(t, "exit with code 3 when more mutants than this survive (config threshold.max_survivors; 0 is off)", flag.Usage)
}

func TestMaxSurvivorsFlag_HasNoDiffCopy(t *testing.T) {
	cmd := &cobra.Command{Use: "x"}
	AddThresholdFlags(cmd, "threshold.diff", "threshold-diff")

	assert.Nil(t, cmd.Flags().Lookup("threshold-diff-max-survivors"), "a --diff run counts its own survivors against threshold.max_survivors")
}

func TestMaxSurvivorsFlag_Invalid(t *testing.T) {
	defer viper.Reset()

	_, err := readMaxSurvivors(t, "--threshold-max-survivors", "-1")
	require.EqualError(t, err, "invalid --threshold-max-survivors: -1 is out of range: must be at least 0")
	assert.Equal(t, m.ExitUsage, codeOf(err))

	viper.Set(config.KeyThresholdMaxSurvivors, -3)
	_, err = readMaxSurvivors(t)
	require.EqualError(t, err, "invalid threshold.max_survivors: -3 is out of range: must be at least 0")
	assert.Equal(t, m.ExitUsage, codeOf(err))

	viper.Set(config.KeyThresholdMaxSurvivors, "lots")
	_, err = readMaxSurvivors(t)
	require.EqualError(t, err, `invalid threshold.max_survivors: "lots" is not a whole number`)
	assert.Equal(t, m.ExitUsage, codeOf(err))

	viper.Set(config.KeyThresholdMaxSurvivors, 2.5)
	_, err = readMaxSurvivors(t)
	require.EqualError(t, err, `invalid threshold.max_survivors: "2.5" is not a whole number`)
}

func TestReportViewAndMergePassMaxSurvivors(t *testing.T) {
	viper.Set(testOutputKey, "/tmp/reports")
	viper.Set(config.KeyThresholdMaxSurvivors, 4)
	defer viper.Reset()

	wf := domainmocks.NewMockWorkflow(t)
	// A --diff run's reports count the survivors on their changed lines, so
	// view checks the same maximum against them.
	wf.On("View", mock.Anything, domain.ViewArgs{
		Reports: "/tmp/reports", Paths: []m.Path{},
		Thresholds:     domain.Thresholds{Key: "threshold", MaxSurvivors: 3},
		DiffThresholds: domain.Thresholds{Key: "threshold.diff", MaxSurvivors: 3},
	}).Return(nil)
	wf.On("Merge", mock.Anything, domain.MergeArgs{Reports: "/tmp/reports", Thresholds: domain.Thresholds{Key: "threshold", MaxSurvivors: 4}}).Return(nil)

	deps := Deps{Workflow: wf, Publisher: domainmocks.NewMockReportPublisher(t), OutputKey: testOutputKey}

	require.NoError(t, execReport(t, deps, "view", "--threshold-max-survivors", "3"))
	require.NoError(t, execReport(t, deps, "merge"))

	err := execReport(t, deps, "merge", "--threshold-max-survivors", "-1")
	assert.Equal(t, m.ExitUsage, codeOf(err))
	wf.AssertNumberOfCalls(t, "Merge", 1)
}

func TestReportViewAndMergePassThresholds(t *testing.T) {
	viper.Set(testOutputKey, "/tmp/reports")
	viper.Set(config.KeyThresholdAverage, 50)
	defer viper.Reset()

	want := domain.Thresholds{Key: "threshold", Total: 75, Average: 50}

	wf := domainmocks.NewMockWorkflow(t)
	wf.On("View", mock.Anything, domain.ViewArgs{Reports: "/tmp/reports", Paths: []m.Path{}, Thresholds: want, DiffThresholds: domain.Thresholds{Key: "threshold.diff"}}).Return(nil)
	wf.On("Merge", mock.Anything, domain.MergeArgs{Reports: "/tmp/reports", Thresholds: want}).Return(nil)

	deps := Deps{Workflow: wf, Publisher: domainmocks.NewMockReportPublisher(t), OutputKey: testOutputKey}

	require.NoError(t, execReport(t, deps, "view", "--threshold-total", "75"))
	require.NoError(t, execReport(t, deps, "merge", "--threshold-total", "75"))

	err := execReport(t, deps, "merge", "--threshold-file", "100.5")
	assert.Equal(t, m.ExitUsage, codeOf(err))
	wf.AssertNumberOfCalls(t, "Merge", 1)
}

func TestReportViewPassesDiffThresholds(t *testing.T) {
	viper.Set(testOutputKey, "/tmp/reports")
	viper.Set(config.KeyThresholdDiffAverage, 60)
	defer viper.Reset()

	wf := domainmocks.NewMockWorkflow(t)
	wf.On("View", mock.Anything, domain.ViewArgs{
		Reports: "/tmp/reports", Paths: []m.Path{},
		Thresholds:     domain.Thresholds{Key: "threshold"},
		DiffThresholds: domain.Thresholds{Key: "threshold.diff", Total: 100, Average: 60},
	}).Return(nil)

	deps := Deps{Workflow: wf, Publisher: domainmocks.NewMockReportPublisher(t), OutputKey: testOutputKey}

	require.NoError(t, execReport(t, deps, "view", "--threshold-diff-total", "100"))

	err := execReport(t, deps, "view", "--threshold-diff-file", "101")
	assert.Equal(t, m.ExitUsage, codeOf(err))
	wf.AssertNumberOfCalls(t, "View", 1)
}

func TestReportMergeHasNoDiffThresholds(t *testing.T) {
	deps := Deps{Workflow: domainmocks.NewMockWorkflow(t), Publisher: domainmocks.NewMockReportPublisher(t), OutputKey: testOutputKey}

	// Merge scores the whole codebase, so only the plain thresholds apply.
	require.Error(t, execReport(t, deps, "merge", "--threshold-diff-total", "100"))
}

// codeOf returns the exit code err carries, or ExitFailure when none.
func codeOf(err error) m.ExitCode {
	var coded *m.CodedError
	if errors.As(err, &coded) {
		return coded.Code
	}

	return m.ExitFailure
}
