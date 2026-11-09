package domain

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	m "gooze.dev/pkg/gooze/internal/model"
)

// mergeScopes merges and shows both scopes of a --diff run: it merges the
// shards in the run dir and shows the changed lines, then folds them into
// the base, keeps the merged run dir and shows the whole project, whose
// metrics add the changed lines'. Auto gates both scopes, each on its own
// thresholds; diff, total and none narrow that.
func (w *workflow) mergeScopes(ctx context.Context, args MergeArgs) error {
	base := args.Reports
	runDir := m.Path(filepath.Join(string(base), RunDirName))

	if err := w.mergeDiffRun(ctx, runDir); err != nil {
		return err
	}

	// Each scope is checked against its own thresholds; --gate leaves one or
	// both out.
	diffView := args.View
	diffView.Reports, diffView.Paths = runDir, nil
	diffView.Thresholds, diffView.DiffThresholds = args.Thresholds, args.DiffThresholds
	// The changed lines' stdout is the text summary; --format is the whole
	// project's.
	diffView.Format, diffView.Output, diffView.Writes = FormatText, nil, args.DiffWrites

	diffScore, diffBreach, err := w.showScope(ctx, diffView)
	if err != nil {
		return err
	}

	if err := w.mergeRunReports(ctx, base, true); err != nil {
		return err
	}

	if err := w.regenerateIndex(ctx, base); err != nil {
		return err
	}

	totalView := diffView
	totalView.Reports, totalView.Format, totalView.Output = base, args.View.Format, args.View.Output
	totalView.Writes, totalView.DiffScore = args.View.Writes, &diffScore.Summary

	_, totalBreach, err := w.showScope(ctx, totalView)
	if err != nil {
		return err
	}

	return errors.Join(diffBreach, totalBreach)
}

// mergeDiffRun merges the shards of the run dir, which must hold a --diff
// run's reports.
func (w *workflow) mergeDiffRun(ctx context.Context, runDir m.Path) error {
	if _, err := os.Stat(string(runDir)); err != nil {
		return noDiffRunError(runDir)
	}

	if err := w.mergeShardDirs(ctx, runDir); err != nil {
		return err
	}

	selection, err := w.reports.LoadSelection(ctx, runDir)
	if err != nil {
		return fmt.Errorf("load the run's selection: %w", err)
	}

	// Equivalent mutant: no ref sorts before "".
	if selection.Diff == "" { //gooze:ignore comparison
		return noDiffRunError(runDir)
	}

	return w.regenerateIndex(ctx, runDir)
}

func noDiffRunError(runDir m.Path) error {
	return WithExitCode(m.ExitNoReports, fmt.Errorf("--scope diff needs the reports of a gooze run --diff in %s", runDir))
}
