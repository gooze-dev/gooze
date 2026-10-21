package domain

import (
	"context"
	"errors"

	m "gooze.dev/pkg/gooze/internal/model"
)

// Errors a ChangeDetector wraps, so callers can tell why --diff can't run.
var (
	// ErrGitNotInstalled means the git binary was not found on PATH.
	ErrGitNotInstalled = errors.New("--diff needs git, but git was not found on PATH")
	// ErrNotGitRepository means the directory is not inside a git work tree.
	ErrNotGitRepository = errors.New("--diff needs a git repository")
	// ErrUnknownRef means the ref, or its merge base with HEAD, can't be
	// resolved; in CI this is usually a shallow clone.
	ErrUnknownRef = errors.New("unknown ref")
)

// ChangeDetector reports which lines of which files changed against a git
// ref, for git diff mode.
type ChangeDetector interface {
	// Changes compares the working tree of the repository containing dir with
	// the merge base of ref and HEAD.
	Changes(ctx context.Context, dir m.Path, ref string) (ChangeSet, error)
}

// ChangeSet is the set of changes against a ref.
type ChangeSet struct {
	// Ref is the ref as given, e.g. "origin/main".
	Ref string
	// Base is the merge base commit of Ref and HEAD.
	Base string
	// Files maps absolute file paths to their changes. Files that did not
	// change, and deleted files, are absent.
	Files map[string]FileChange
}

// FileChange describes the changed lines of one file.
type FileChange struct {
	// Whole marks a file that counts as changed in full, such as a new or
	// untracked file.
	Whole bool
	// Lines are the added or modified line ranges, in the file's current
	// content, when Whole is false.
	Lines []LineRange
}

// LineRange is an inclusive range of 1-based line numbers.
type LineRange struct {
	Start int
	End   int
}

// Contains reports whether line is changed.
func (c FileChange) Contains(line int) bool {
	if c.Whole {
		return true
	}

	for _, r := range c.Lines {
		if line >= r.Start && line <= r.End {
			return true
		}
	}

	return false
}
