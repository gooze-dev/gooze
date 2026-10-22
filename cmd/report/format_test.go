package report

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
	m "gooze.dev/pkg/gooze/internal/model"
	"gooze.dev/pkg/gooze/pkg/config"
)

// stubShowPrefix makes the git prefix lookup return prefix, or fail when err is
// set, for one test.
func stubShowPrefix(t *testing.T, prefix string, err error) {
	t.Helper()

	original := showPrefix
	showPrefix = func(context.Context) (string, error) { return prefix, err }

	t.Cleanup(func() { showPrefix = original })
}

// viewArgsFor runs report view with args and returns the ViewArgs it passed.
func viewArgsFor(t *testing.T, args ...string) domain.ViewArgs {
	t.Helper()

	var got domain.ViewArgs

	wf := domainmocks.NewMockWorkflow(t)
	wf.On("View", mock.Anything, mock.Anything).Run(func(a mock.Arguments) { got = a.Get(1).(domain.ViewArgs) }).Return(nil)

	deps := Deps{Workflow: wf, Publisher: domainmocks.NewMockReportPublisher(t), OutputKey: testOutputKey}
	require.NoError(t, execReport(t, deps, append([]string{"view"}, args...)...))

	return got
}

func TestReportView_FormatGitHub(t *testing.T) {
	viper.Set(testOutputKey, "/tmp/reports")
	defer viper.Reset()

	stubShowPrefix(t, "sub/mod/", nil)

	got := viewArgsFor(t, "--format", "github")

	assert.Equal(t, domain.FormatGitHub, got.Format)
	assert.Equal(t, "sub/mod/", got.PathPrefix)
	assert.NotNil(t, got.Output)
	assert.Equal(t, m.Path("/tmp/reports"), got.Reports)
}

func TestReportView_FormatFromConfig(t *testing.T) {
	viper.Set(testOutputKey, "/tmp/reports")
	viper.Set(config.KeyReportFormat, "github")
	defer viper.Reset()

	stubShowPrefix(t, "", nil)

	assert.Equal(t, domain.FormatGitHub, viewArgsFor(t).Format)
}

func TestReportView_FormatFlagOverridesConfig(t *testing.T) {
	viper.Set(testOutputKey, "/tmp/reports")
	viper.Set(config.KeyReportFormat, "github")
	defer viper.Reset()

	got := viewArgsFor(t, "--format", "text")

	assert.Empty(t, got.Format, "text leaves the view as before")
	assert.Nil(t, got.Output)
}

func TestReportView_TextFormatPassesNoOutput(t *testing.T) {
	viper.Set(testOutputKey, "/tmp/reports")
	viper.Set(config.KeyReportFormat, "text")
	defer viper.Reset()

	got := viewArgsFor(t)

	assert.Empty(t, got.Format)
	assert.Nil(t, got.Output)
	assert.Empty(t, got.PathPrefix)
}

func TestReportView_RejectsUnknownFormat(t *testing.T) {
	viper.Set(testOutputKey, "/tmp/reports")
	defer viper.Reset()

	deps := Deps{Workflow: domainmocks.NewMockWorkflow(t), Publisher: domainmocks.NewMockReportPublisher(t), OutputKey: testOutputKey}
	err := execReport(t, deps, "view", "--format", "xml")

	require.EqualError(t, err, `invalid --format "xml": must be one of text, github`)
	assert.Equal(t, m.ExitUsage, codeOf(err))
}

func TestReportView_AllWithFormatFlagIsAUsageError(t *testing.T) {
	viper.Set(testOutputKey, "/tmp/reports")
	defer viper.Reset()

	deps := Deps{Workflow: domainmocks.NewMockWorkflow(t), Publisher: domainmocks.NewMockReportPublisher(t), OutputKey: testOutputKey}
	err := execReport(t, deps, "view", "--all", "--format", "github")

	require.EqualError(t, err, "--all lists every mutation as text; it can't be combined with --format github")
	assert.Equal(t, m.ExitUsage, codeOf(err))
}

func TestReportView_AllIgnoresConfiguredFormat(t *testing.T) {
	viper.Set(testOutputKey, "/tmp/reports")
	viper.Set(config.KeyReportFormat, "github")
	defer viper.Reset()

	got := viewArgsFor(t, "--all")

	assert.True(t, got.All)
	assert.Empty(t, got.Format)
}

func TestRepoPathPrefix(t *testing.T) {
	ctx := context.Background()

	t.Run("from git", func(t *testing.T) {
		stubShowPrefix(t, "mod/", nil)
		assert.Equal(t, "mod/", repoPathPrefix(ctx))
	})

	t.Run("from GITHUB_WORKSPACE outside a git repo", func(t *testing.T) {
		stubShowPrefix(t, "", errors.New("not a git repository"))

		workspace := t.TempDir()
		module := filepath.Join(workspace, "tools", "lint")
		require.NoError(t, os.MkdirAll(module, 0o755))
		t.Chdir(module)
		t.Setenv("GITHUB_WORKSPACE", workspace)

		assert.Equal(t, "tools/lint/", repoPathPrefix(ctx))
	})

	t.Run("at the workspace root", func(t *testing.T) {
		stubShowPrefix(t, "", errors.New("not a git repository"))

		workspace := t.TempDir()
		t.Chdir(workspace)
		t.Setenv("GITHUB_WORKSPACE", workspace)

		assert.Empty(t, repoPathPrefix(ctx))
	})

	t.Run("outside the workspace", func(t *testing.T) {
		stubShowPrefix(t, "", errors.New("not a git repository"))

		t.Chdir(t.TempDir())
		t.Setenv("GITHUB_WORKSPACE", t.TempDir())

		assert.Empty(t, repoPathPrefix(ctx))
	})

	t.Run("no git and no workspace", func(t *testing.T) {
		stubShowPrefix(t, "", errors.New("not a git repository"))
		t.Setenv("GITHUB_WORKSPACE", "")

		assert.Empty(t, repoPathPrefix(ctx))
	})
}

func TestRepoPathPrefix_DirectorySortingBeforeDot(t *testing.T) {
	stubShowPrefix(t, "", errors.New("not a git repository"))

	workspace := t.TempDir()
	module := filepath.Join(workspace, "-tools")
	require.NoError(t, os.MkdirAll(module, 0o755))
	t.Chdir(module)
	t.Setenv("GITHUB_WORKSPACE", workspace)

	assert.Equal(t, "-tools/", repoPathPrefix(context.Background()))
}

// gitEnv runs git without the variables a git hook sets, so it uses the
// repository of its working directory.
func gitEnv(t *testing.T) {
	t.Helper()

	for _, name := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR"} {
		t.Setenv(name, "")
		require.NoError(t, os.Unsetenv(name))
	}
}

func TestShowPrefix(t *testing.T) {
	gitEnv(t)

	t.Run("in a repository subdirectory", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, exec.Command("git", "init", "-q", root).Run())

		sub := filepath.Join(root, "tools", "lint")
		require.NoError(t, os.MkdirAll(sub, 0o755))
		t.Chdir(sub)

		prefix, err := showPrefix(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "tools/lint/", prefix)
	})

	t.Run("outside a repository", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
		t.Chdir(dir)

		_, err := showPrefix(context.Background())
		assert.Error(t, err)
	})
}
