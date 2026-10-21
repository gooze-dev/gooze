package domain

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	pkg "gooze.dev/pkg/gooze/pkg"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// DiffInfo describes a diff-mode run for display: the ref as given, its merge
// base with HEAD, and how many source files are in scope.
type DiffInfo struct {
	Ref   string
	Base  string
	Files int
}

// DiffScope decides which sources and lines a diff-mode run mutates. A nil
// scope means diff mode is off and everything is in scope.
type DiffScope struct {
	changes ChangeSet
}

// NewDiffScope returns the scope of the given changes.
func NewDiffScope(changes ChangeSet) *DiffScope {
	return &DiffScope{changes: changes}
}

// Info returns the ref and merge base of the scope.
func (s *DiffScope) Info() DiffInfo {
	return DiffInfo{Ref: s.changes.Ref, Base: s.changes.Base}
}

// InScope reports whether any mutation of source can be in scope: its file
// has changed lines, or the test file paired with it changed.
func (s *DiffScope) InScope(source rpt.Source) bool {
	if s == nil {
		return true
	}

	if source.Origin == nil {
		return false
	}

	if s.testChanged(source) {
		return true
	}

	change, ok := s.changes.Files[source.Origin.FullPath]

	return ok && (change.Whole || len(change.Lines) > 0)
}

// Includes reports whether a mutation of source on line is in scope. A
// changed test file puts the whole source it tests in scope, since new or
// edited tests can kill mutants anywhere in it.
func (s *DiffScope) Includes(source rpt.Source, line int) bool {
	if s == nil {
		return true
	}

	if source.Origin == nil {
		return false
	}

	if s.testChanged(source) {
		return true
	}

	return s.changes.Files[source.Origin.FullPath].Contains(line)
}

// ReportScope returns what reports of source record about the lines tested:
// nil when the whole file is in scope (diff mode off, a new file, or a changed
// test), otherwise the base and changed line ranges.
func (s *DiffScope) ReportScope(source rpt.Source) *rpt.Scope {
	if s == nil || source.Origin == nil || s.testChanged(source) {
		return nil
	}

	change := s.changes.Files[source.Origin.FullPath]
	if change.Whole {
		return nil
	}

	lines := make([]rpt.LineRange, 0, len(change.Lines))
	for _, r := range change.Lines {
		lines = append(lines, rpt.LineRange{Start: r.Start, End: r.End})
	}

	return &rpt.Scope{Base: s.changes.Base, Lines: lines}
}

func (s *DiffScope) testChanged(source rpt.Source) bool {
	if source.Test == nil {
		return false
	}

	_, ok := s.changes.Files[source.Test.FullPath]

	return ok
}

// errNoChangeDetector is returned for --diff when the workflow was built
// without a ChangeDetector.
var errNoChangeDetector = errors.New("diff mode is not available: no change detector configured")

// WorkflowOption configures optional workflow dependencies.
type WorkflowOption func(*workflow)

// WithChangeDetector enables diff mode with the given change detector.
func WithChangeDetector(detector ChangeDetector) WorkflowOption {
	return func(w *workflow) { w.changes = detector }
}

// diffScope resolves the scope of args.Diff, or nil when diff mode is off.
// Changes are read relative to the working directory, like source paths.
func (w *workflow) diffScope(ctx context.Context, args EstimateArgs) (*DiffScope, error) {
	if args.Diff == "" {
		return nil, nil //nolint:nilnil // a nil scope means diff mode is off.
	}

	if w.changes == nil {
		return nil, errNoChangeDetector
	}

	changes, err := w.changes.Changes(ctx, ".", args.Diff)
	if err != nil {
		return nil, fmt.Errorf("diff against %s: %w", args.Diff, err)
	}

	return NewDiffScope(changes), nil
}

// narrowToScope keeps only the changed sources in diff scope, so a diff run
// neither tests nor cleans the stored reports of sources outside it.
func narrowToScope(sources sourceSet, scope *DiffScope) (sourceSet, error) {
	if scope == nil {
		return sources, nil
	}

	inScope, err := pkg.NewFileSpill[rpt.Source]()
	if err != nil {
		return sources, fmt.Errorf("create diff scope spill: %w", err)
	}

	err = sources.changed.Range(func(_ uint64, source rpt.Source) error {
		if !scope.InScope(source) {
			return nil
		}

		return inScope.Append(source)
	})
	if err != nil {
		closeSpill("diff scope sources", inScope)
		return sources, err
	}

	// Equivalent: when changed is scanned, closing it early is never observed,
	// since scanned is read again only by cached runs, where they differ.
	if sources.changed != sources.scanned { //gooze:ignore comparison
		closeSpill("changed sources", sources.changed)
	}

	sources.changed = inScope

	return sources, nil
}

// testSources resolves a test run's selection and sources, narrowing the
// sources to test to those in diff scope. Selection errors (a bad pattern,
// an unknown mutagen, git) are returned before any scan. The caller must
// close the set.
func (w *workflow) testSources(ctx context.Context, args EstimateArgs) (sourceSet, Selection, error) {
	sel, err := w.selection(ctx, args)
	if err != nil {
		slog.Error("Failed to resolve the run's selection", "error", err)
		return sourceSet{}, Selection{}, err
	}

	sources, err := w.resolveSources(ctx, args)
	if err != nil {
		slog.Error("Failed to resolve sources", "error", err)
		return sourceSet{}, Selection{}, err
	}

	narrowed, err := narrowToScope(sources, sel.Diff)
	if err != nil {
		sources.close()
		return sourceSet{}, Selection{}, err
	}

	return narrowed, sel, nil
}
