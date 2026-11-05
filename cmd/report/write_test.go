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

func TestParseWrite(t *testing.T) {
	format, path, err := parseWrite("metrics=out/metrics.txt")
	require.NoError(t, err)
	assert.Equal(t, domain.FormatMetrics, format)
	assert.Equal(t, "out/metrics.txt", path)

	_, path, err = parseWrite("text=a=b.txt")
	require.NoError(t, err)
	assert.Equal(t, "a=b.txt", path, "the path is everything after the first =")

	for value, want := range map[string]string{
		"metrics":   `--write needs <format>=<path>, got "metrics"`,
		"metrics=":  `--write needs <format>=<path>, got "metrics="`,
		"=out.txt":  `--write needs <format>=<path>, got "=out.txt"`,
		"html=a":    `invalid --write format "html": must be one of text, gitlab, sarif, codeclimate, metrics`,
		"github=a":  `invalid --write format "github": must be one of text, gitlab, sarif, codeclimate, metrics`,
		"TEXT=a.md": `invalid --write format "TEXT": must be one of text, gitlab, sarif, codeclimate, metrics`,
	} {
		_, _, err := parseWrite(value)
		require.EqualError(t, err, want, value)
		assert.Equal(t, m.ExitUsage, codeOf(err), value)
	}
}

func TestReportView_WriteRepeats(t *testing.T) {
	stubShowPrefix(t, "sub/", nil)

	dir := t.TempDir()
	metrics, text := filepath.Join(dir, "metrics.txt"), filepath.Join(dir, "report.txt")

	args := viewArgsFor(t, "--write", "metrics="+metrics, "--write", "text="+text)

	require.Len(t, args.Writes, 2)
	assert.Equal(t, domain.FormatMetrics, args.Writes[0].Format)
	assert.Equal(t, domain.FormatText, args.Writes[1].Format)
	assert.Equal(t, "sub/", args.PathPrefix, "files get repository paths with a text stdout too")
	assert.NotEmpty(t, args.ToolVersion)

	for _, path := range []string{metrics, text} {
		_, err := os.Stat(path)
		require.NoError(t, err, "the files are created")
	}
}

func TestReportView_OneWriteGetsRepositoryPaths(t *testing.T) {
	stubShowPrefix(t, "sub/", nil)

	args := viewArgsFor(t, "--write", "gitlab="+filepath.Join(t.TempDir(), "cq.json"))

	require.Len(t, args.Writes, 1)
	assert.Equal(t, "sub/", args.PathPrefix)
}

func TestReportView_WriteWithAll(t *testing.T) {
	dir := t.TempDir()
	deps := Deps{Workflow: domainmocks.NewMockWorkflow(t), Publisher: domainmocks.NewMockReportPublisher(t), OutputKey: testOutputKey}

	err := execReport(t, deps, "view", "--all", "--write", "metrics="+filepath.Join(dir, "m.txt"))
	require.EqualError(t, err, "--write writes the summary; --all lists every mutation")
	assert.Equal(t, m.ExitUsage, codeOf(err))

	entries, readErr := os.ReadDir(dir)
	require.NoError(t, readErr)
	assert.Empty(t, entries)
}

func TestReportView_WriteUsageErrorCreatesNothing(t *testing.T) {
	dir := t.TempDir()
	deps := Deps{Workflow: domainmocks.NewMockWorkflow(t), Publisher: domainmocks.NewMockReportPublisher(t), OutputKey: testOutputKey}

	err := execReport(t, deps, "view", "--write", "metrics="+filepath.Join(dir, "m.txt"), "--write", "html="+filepath.Join(dir, "h.html"))
	require.Error(t, err)

	entries, readErr := os.ReadDir(dir)
	require.NoError(t, readErr)
	assert.Empty(t, entries, "a bad --write value creates no file")
}

func TestReportMerge_ShowsTheReportLikeView(t *testing.T) {
	t.Setenv("GITLAB_CI", "true")
	stubShowPrefix(t, "", nil)
	viper.Set(testOutputKey, "/tmp/reports")
	defer viper.Reset()

	var got domain.MergeArgs

	wf := domainmocks.NewMockWorkflow(t)
	wf.On("Merge", mock.Anything, mock.Anything).Run(func(a mock.Arguments) { got = a.Get(1).(domain.MergeArgs) }).Return(nil)

	deps := Deps{Workflow: wf, Publisher: domainmocks.NewMockReportPublisher(t), OutputKey: testOutputKey}
	metrics := filepath.Join(t.TempDir(), "metrics.txt")

	require.NoError(t, execReport(t, deps, "merge", "--summary", "--top", "3", "--by-line=false", "--write", "metrics="+metrics))

	assert.Equal(t, m.Path("/tmp/reports"), got.Reports)
	assert.True(t, got.View.Summary)
	assert.Equal(t, 3, got.View.Top)
	assert.True(t, got.View.Sections)
	require.Len(t, got.View.Writes, 1)
	assert.Equal(t, domain.FormatMetrics, got.View.Writes[0].Format)
}

func TestReportMerge_RejectsAll(t *testing.T) {
	deps := Deps{Workflow: domainmocks.NewMockWorkflow(t), Publisher: domainmocks.NewMockReportPublisher(t), OutputKey: testOutputKey}

	require.ErrorContains(t, execReport(t, deps, "merge", "--all"), "unknown flag: --all")
}
