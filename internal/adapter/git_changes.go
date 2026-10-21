package adapter

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
)

// GitChangeDetector finds the lines changed against a ref by running the git
// binary. It compares the working tree (committed, staged, unstaged and
// untracked files) with the merge base of the ref and HEAD.
type GitChangeDetector struct {
	// LookPath finds the git binary; exec.LookPath by default.
	LookPath func(file string) (string, error)
}

// NewGitChangeDetector returns a GitChangeDetector that uses git from PATH.
func NewGitChangeDetector() *GitChangeDetector {
	return &GitChangeDetector{LookPath: exec.LookPath}
}

// Changes implements domain.ChangeDetector. Returned paths are absolute and
// rooted at dir as given (not symlink-resolved), so they match the source
// paths gooze builds from the same directory.
func (d *GitChangeDetector) Changes(ctx context.Context, dir m.Path, ref string) (domain.ChangeSet, error) {
	git, err := d.LookPath("git")
	if err != nil {
		return domain.ChangeSet{}, fmt.Errorf("%w: %w", domain.ErrGitNotInstalled, err)
	}

	abs, err := filepath.Abs(string(dir))
	if err != nil {
		return domain.ChangeSet{}, err
	}

	prefix, err := runGit(ctx, git, abs, "rev-parse", "--show-prefix")
	if err != nil {
		return domain.ChangeSet{}, fmt.Errorf("%w: %w", domain.ErrNotGitRepository, err)
	}

	top := worktreeTop(abs, prefix)

	base, err := mergeBase(ctx, git, top, ref)
	if err != nil {
		return domain.ChangeSet{}, err
	}

	// Pin the output format against user and repo config: path prefixes,
	// diff.relative, external diff tools, textconv filters and color.
	diff, err := runGit(ctx, git, top, "diff", "--no-color", "--no-ext-diff", "--no-textconv", "--no-relative",
		"--src-prefix=a/", "--dst-prefix=b/", "-U0", "-M", base, "--")
	if err != nil {
		return domain.ChangeSet{}, err
	}

	untracked, err := runGit(ctx, git, top, "ls-files", "--others", "--exclude-standard", "--full-name", "-z")
	if err != nil {
		return domain.ChangeSet{}, err
	}

	files := make(map[string]domain.FileChange)

	for rel, change := range parseUnifiedDiff(diff) {
		files[filepath.Join(top, filepath.FromSlash(rel))] = change
	}

	for rel := range strings.SplitSeq(untracked, "\x00") {
		if rel != "" {
			files[filepath.Join(top, filepath.FromSlash(rel))] = domain.FileChange{Whole: true}
		}
	}

	return domain.ChangeSet{Ref: ref, Base: base, Files: files}, nil
}

// mergeBase resolves ref and returns its merge base with HEAD.
func mergeBase(ctx context.Context, git, top, ref string) (string, error) {
	if _, err := runGit(ctx, git, top, "rev-parse", "--verify", "--quiet", ref+"^{commit}"); err != nil {
		return "", fmt.Errorf("%w %q; fetch it first (actions/checkout: fetch-depth: 0, or git fetch origin <branch>)", domain.ErrUnknownRef, ref)
	}

	base, err := runGit(ctx, git, top, "merge-base", ref, "HEAD")
	if err != nil {
		return "", fmt.Errorf("%w: no merge base between %q and HEAD; fetch the full history (actions/checkout: fetch-depth: 0, or git fetch --unshallow)", domain.ErrUnknownRef, ref)
	}

	return strings.TrimSpace(base), nil
}

// worktreeTop returns the work tree root above abs, given git's
// --show-prefix output (e.g. "pkg/calc/\n"), without resolving symlinks the
// way --show-toplevel does.
func worktreeTop(abs, prefix string) string {
	top := abs

	for part := range strings.SplitSeq(strings.TrimSpace(prefix), "/") {
		if part != "" {
			top = filepath.Dir(top)
		}
	}

	return top
}

// runGit runs git in dir and returns its stdout. A failure includes the first
// line of git's stderr.
func runGit(ctx context.Context, git, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, git, append([]string{"-c", "core.quotepath=off"}, args...)...) //nolint:gosec // fixed git subcommands; ref is passed as one argument.
	cmd.Dir = dir
	// No optional index refresh writes, and stable English messages.
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "LC_ALL=C")

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg, _, _ := strings.Cut(strings.TrimSpace(stderr.String()), "\n")
		if msg == "" {
			return "", fmt.Errorf("git %s: %w", args[0], err)
		}

		return "", fmt.Errorf("git %s: %s", args[0], msg)
	}

	return stdout.String(), nil
}

// parseUnifiedDiff returns the added or modified lines of each file in a
// `git diff -U0` output, keyed by repository-relative path. New files count
// in full; a file whose hunks only delete lines is listed with no lines;
// deleted files and renames without edits are absent.
func parseUnifiedDiff(diff string) map[string]domain.FileChange {
	files := make(map[string]domain.FileChange)

	var (
		current string
		newFile bool
	)

	for line := range strings.SplitSeq(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			current, newFile = "", false
		case strings.HasPrefix(line, "new file mode"):
			newFile = true
		case strings.HasPrefix(line, "+++ "):
			current = diffPath(strings.TrimPrefix(line, "+++ "))
			if current != "" {
				// Listed even if its hunks only delete lines: a test file
				// changed that way still puts its source in scope.
				files[current] = domain.FileChange{Whole: newFile}
			}
		// Equivalent: a deleted file ("") only has hunks with zero new lines,
		// which addHunk skips anyway.
		case strings.HasPrefix(line, "@@ ") && current != "" && !newFile: //gooze:ignore comparison
			addHunk(files, current, line)
		}
	}

	return files
}

// addHunk records the new-side lines of a hunk header for path; a hunk that
// only deletes lines adds none.
func addHunk(files map[string]domain.FileChange, path, header string) {
	start, count, ok := parseHunkNewRange(header)
	// Equivalent: git never writes a negative line count.
	if !ok || count == 0 { //gooze:ignore comparison
		return
	}

	change := files[path]
	change.Lines = append(change.Lines, domain.LineRange{Start: start, End: start + count - 1})
	files[path] = change
}

// diffPath turns a "+++" path ("b/calc.go", a C-quoted path, or /dev/null)
// into a repository-relative path, or "" for a deleted file.
func diffPath(raw string) string {
	if raw == "/dev/null" {
		return ""
	}

	if strings.HasPrefix(raw, `"`) {
		if unquoted, err := strconv.Unquote(raw); err == nil {
			raw = unquoted
		}
	}

	return strings.TrimPrefix(raw, "b/")
}

// parseHunkNewRange reads the new-file range of a hunk header such as
// "@@ -3,2 +5,4 @@": start 5, count 4. A missing count means 1.
func parseHunkNewRange(header string) (start, count int, ok bool) {
	_, rest, found := strings.Cut(header, " +")
	if !found {
		return 0, 0, false
	}

	field, _, _ := strings.Cut(rest, " ")
	startText, countText, hasCount := strings.Cut(field, ",")

	start, err := strconv.Atoi(startText)
	if err != nil {
		return 0, 0, false
	}

	if !hasCount {
		return start, 1, true
	}

	count, err = strconv.Atoi(countText)
	if err != nil {
		return 0, 0, false
	}

	return start, count, true
}
