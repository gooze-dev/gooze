package adapter

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
)

// gitRepo is a temporary git repository for change-detection tests.
type gitRepo struct {
	t   *testing.T
	dir string
}

func newGitRepo(t *testing.T) *gitRepo {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}

	r := &gitRepo{t: t, dir: t.TempDir()}
	r.git("init", "-q", "-b", "main")

	return r
}

func (r *gitRepo) git(args ...string) string {
	r.t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = r.dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
		"GIT_CONFIG_NOSYSTEM=1", "HOME="+r.dir,
	)

	out, err := cmd.CombinedOutput()
	require.NoError(r.t, err, "git %s: %s", strings.Join(args, " "), out)

	return strings.TrimSpace(string(out))
}

func (r *gitRepo) write(rel, content string) {
	r.t.Helper()

	path := filepath.Join(r.dir, rel)
	require.NoError(r.t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(r.t, os.WriteFile(path, []byte(content), 0o600))
}

func (r *gitRepo) commit(msg string) {
	r.t.Helper()
	r.git("add", "-A")
	r.git("commit", "-q", "-m", msg)
}

func (r *gitRepo) path(rel string) string { return filepath.Join(r.dir, rel) }

func lines(n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		b.WriteString("line\n")
	}

	return b.String()
}

// withLine returns n "line" lines with line k replaced by text.
func withLine(n, k int, text string) string {
	return withLines(n, map[int]string{k: text})
}

// withLines returns n "line" lines with the given line numbers replaced.
func withLines(n int, replaced map[int]string) string {
	out := strings.Split(strings.TrimSuffix(lines(n), "\n"), "\n")
	for k, text := range replaced {
		out[k-1] = text
	}

	return strings.Join(out, "\n") + "\n"
}

func TestGitChangeDetector_WorkingTreeAgainstMergeBase(t *testing.T) {
	r := newGitRepo(t)
	r.write("calc.go", lines(10))
	r.write("other.go", lines(5))
	r.write("gone.go", lines(3))
	r.write("same.go", lines(4))
	r.write(".gitignore", "ignored.go\n")
	r.commit("base")

	r.git("checkout", "-q", "-b", "feature")
	r.write("calc.go", withLine(10, 3, "changed"))
	require.NoError(t, os.Remove(r.path("gone.go")))
	r.git("mv", "same.go", "moved.go")
	r.commit("feature work")

	// main moves on after the branch point; its changes are not ours.
	r.git("checkout", "-q", "main")
	r.write("other.go", withLine(5, 1, "main only"))
	r.commit("main moves on")
	r.git("checkout", "-q", "feature")

	// Uncommitted work counts too: unstaged, staged and untracked files.
	r.write("calc.go", withLines(10, map[int]string{3: "changed", 5: "edited"}))
	// Distinct content, so git doesn't pair them with gone.go as a rename.
	r.write("staged.go", "package calc\n\nfunc Staged() {}\n")
	r.git("add", "staged.go")
	r.write("untracked.go", "package calc\n\nfunc Untracked() {}\n")
	r.write("ignored.go", lines(3))

	base := r.git("merge-base", "main", "HEAD")

	got, err := NewGitChangeDetector().Changes(context.Background(), m.Path(r.dir), "main")
	require.NoError(t, err)

	assert.Equal(t, "main", got.Ref)
	assert.Equal(t, base, got.Base)
	assert.Equal(t, map[string]domain.FileChange{
		r.path("calc.go"):      {Lines: []domain.LineRange{{Start: 3, End: 3}, {Start: 5, End: 5}}},
		r.path("staged.go"):    {Whole: true},
		r.path("untracked.go"): {Whole: true},
	}, got.Files)
}

func TestGitChangeDetector_RenameWithEditsCountsOnlyEditedLines(t *testing.T) {
	r := newGitRepo(t)
	r.write("old.go", lines(10))
	r.commit("base")
	r.git("mv", "old.go", "new.go")
	r.write("new.go", withLine(10, 4, "edited"))

	got, err := NewGitChangeDetector().Changes(context.Background(), m.Path(r.dir), "HEAD")
	require.NoError(t, err)
	assert.Equal(t, map[string]domain.FileChange{
		r.path("new.go"): {Lines: []domain.LineRange{{Start: 4, End: 4}}},
	}, got.Files)
}

func TestGitChangeDetector_FromSubdirectoryAndSymlink(t *testing.T) {
	r := newGitRepo(t)
	r.write("pkg/calc/calc.go", lines(3))
	r.commit("base")
	r.write("pkg/calc/calc.go", withLine(3, 2, "changed"))

	link := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(r.dir, link))

	// Paths come back under the directory as given, so they match the source
	// paths gooze builds from the same working directory.
	got, err := NewGitChangeDetector().Changes(context.Background(), m.Path(filepath.Join(link, "pkg")), "HEAD")
	require.NoError(t, err)
	assert.Equal(t, map[string]domain.FileChange{
		filepath.Join(link, "pkg/calc/calc.go"): {Lines: []domain.LineRange{{Start: 2, End: 2}}},
	}, got.Files)
}

func TestGitChangeDetector_NoChanges(t *testing.T) {
	r := newGitRepo(t)
	r.write("calc.go", lines(3))
	r.commit("base")

	got, err := NewGitChangeDetector().Changes(context.Background(), m.Path(r.dir), "HEAD")
	require.NoError(t, err)
	assert.Empty(t, got.Files)
}

func TestGitChangeDetector_UnknownRef(t *testing.T) {
	r := newGitRepo(t)
	r.write("calc.go", lines(3))
	r.commit("base")

	_, err := NewGitChangeDetector().Changes(context.Background(), m.Path(r.dir), "origin/nope")
	require.ErrorIs(t, err, domain.ErrUnknownRef)
	assert.Contains(t, err.Error(), `"origin/nope"`)
	assert.Contains(t, err.Error(), "fetch-depth: 0")
}

func TestGitChangeDetector_NoMergeBase(t *testing.T) {
	r := newGitRepo(t)
	r.write("calc.go", lines(3))
	r.commit("base")
	r.git("checkout", "-q", "--orphan", "unrelated")
	r.commit("unrelated history")

	_, err := NewGitChangeDetector().Changes(context.Background(), m.Path(r.dir), "main")
	require.ErrorIs(t, err, domain.ErrUnknownRef)
	assert.Contains(t, err.Error(), "no merge base")
}

func TestGitChangeDetector_NotARepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}

	dir := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))

	_, err := NewGitChangeDetector().Changes(context.Background(), m.Path(dir), "main")
	require.ErrorIs(t, err, domain.ErrNotGitRepository)
}

func TestGitChangeDetector_GitNotInstalled(t *testing.T) {
	detector := NewGitChangeDetector()
	detector.LookPath = func(string) (string, error) { return "", exec.ErrNotFound }

	_, err := detector.Changes(context.Background(), m.Path(t.TempDir()), "main")
	require.ErrorIs(t, err, domain.ErrGitNotInstalled)
	assert.True(t, errors.Is(err, exec.ErrNotFound))
}

func TestGitChangeDetector_GitNotOnPath(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	_, err := NewGitChangeDetector().Changes(context.Background(), m.Path(t.TempDir()), "main")
	require.ErrorIs(t, err, domain.ErrGitNotInstalled)
}
