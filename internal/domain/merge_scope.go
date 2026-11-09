package domain

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	m "gooze.dev/pkg/gooze/internal/model"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// mergeScopes merges and shows both scopes of a --diff run: it merges the
// shards in the run dir and shows the changed lines, then folds them into
// the base, keeps the merged run dir and shows the whole project, whose
// metrics add the changed lines'. Auto gates both scopes, each on its own
// thresholds; diff, total and none narrow that.
func (w *workflow) mergeScopes(ctx context.Context, args MergeArgs) error {
	base := args.Reports
	runDir := m.Path(filepath.Join(string(base), RunDirName))

	selection, err := w.mergeDiffRun(ctx, runDir)
	if err != nil {
		return err
	}

	// Each scope is checked against its own thresholds; --gate leaves one or
	// both out.
	diffView := args.View
	diffView.Reports, diffView.Paths = runDir, nil
	diffView.Scope = ReportScope{Name: string(GateDiff), Title: "Changed lines since " + selection.Diff}
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

	// The changed lines already list their survivors; the whole project shows
	// its table and score.
	totalView := diffView
	totalView.Reports, totalView.Format, totalView.Output = base, args.View.Format, args.View.Output
	totalView.Summary = true
	totalView.Scope = ReportScope{Name: string(GateTotal), Title: "Whole project", Selection: "whole project"}
	totalView.Writes, totalView.DiffScore = args.View.Writes, &diffScore.Summary

	_, totalBreach, err := w.showScope(ctx, totalView)
	if err != nil {
		return err
	}

	return errors.Join(diffBreach, totalBreach)
}

// mergeDiffRun merges the shards of the run dir, which must hold a --diff
// run's reports, and returns the run's selection.
func (w *workflow) mergeDiffRun(ctx context.Context, runDir m.Path) (rpt.Selection, error) {
	if _, err := os.Stat(string(runDir)); err != nil {
		return rpt.Selection{}, noDiffRunError(runDir)
	}

	if err := w.mergeShardDirs(ctx, runDir); err != nil {
		return rpt.Selection{}, err
	}

	selection, err := w.reports.LoadSelection(ctx, runDir)
	if err != nil {
		return rpt.Selection{}, fmt.Errorf("load the run's selection: %w", err)
	}

	// Equivalent mutant: no ref sorts before "".
	if selection.Diff == "" { //gooze:ignore comparison
		return rpt.Selection{}, noDiffRunError(runDir)
	}

	return selection, w.regenerateIndex(ctx, runDir)
}

func noDiffRunError(runDir m.Path) error {
	return WithExitCode(m.ExitNoReports, fmt.Errorf("--scope diff needs the reports of a gooze run --diff in %s", runDir))
}
