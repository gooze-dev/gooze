package domain

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	m "gooze.dev/pkg/gooze/internal/model"
	pkg "gooze.dev/pkg/gooze/pkg"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// ReportSummary is the outcome of a set of reports per source file, with the
// mutations that lower its score.
type ReportSummary struct {
	Score     MutationScore
	Files     []FileSummary // sorted by path
	Survivors []Survivor    // sorted by path, then line
}

// FileSummary counts the mutation results of one source file.
type FileSummary struct {
	Path string
	rpt.Summary
}

// Survivor is a mutation that counts against the score: it survived, was not
// covered by any test, timed out or was skipped.
type Survivor struct {
	Path     string
	Line     int // one-based; 0 when the report has no diff
	Mutagen  string
	Status   rpt.TestStatus
	Original string // the original line, trimmed
	Mutated  string // the mutated line, trimmed; "" when the line was removed
	// MutationID identifies the mutation across runs.
	MutationID string
}

func (w *workflow) View(ctx context.Context, args ViewArgs) error {
	if args.All {
		return w.viewAll(ctx, args)
	}

	slog.Info("Summarizing mutation test reports", "path", args.Reports, "paths", args.Paths)

	reports, err := w.loadReports(ctx, args.Reports)
	if err != nil {
		return err
	}

	defer closeSpill("reports", reports)

	summary, err := summarizeView(reports, args.Paths)
	if err != nil {
		return fmt.Errorf("summarize reports: %w", err)
	}

	if len(summary.Files) == 0 && len(args.Paths) > 0 {
		return WithExitCode(m.ExitNoReports, fmt.Errorf("no reports in %s match %s", args.Reports, joinPaths(args.Paths)))
	}

	if err := w.displayView(ctx, args, summary); err != nil {
		return err
	}

	thresholds, err := w.viewThresholds(ctx, args)
	if err != nil {
		return err
	}

	return thresholds.Err(summary.Score)
}

// viewThresholds returns the thresholds to check the viewed reports against:
// the diff ones when a --diff run wrote them, the plain ones otherwise.
func (w *workflow) viewThresholds(ctx context.Context, args ViewArgs) (Thresholds, error) {
	selection, err := w.reports.LoadSelection(ctx, args.Reports)
	if err != nil {
		return Thresholds{}, fmt.Errorf("load the run's selection: %w", err)
	}

	if selection.Diff != "" {
		return args.DiffThresholds, nil
	}

	return args.Thresholds, nil
}

// viewAll replays every mutation with its status, as a run displays them.
func (w *workflow) viewAll(ctx context.Context, args ViewArgs) error {
	return w.withTestUI(ctx, func() error {
		slog.Info("Loading mutation test reports", "path", args.Reports)

		reports, err := w.loadReports(ctx, args.Reports)
		if err != nil {
			return err
		}

		defer closeSpill("reports", reports)

		include := func(source rpt.Source) bool { return matchesPaths(source, args.Paths) }
		total := 0

		if err := rangeViewItems(reports, include, func(m.Mutation, m.Result) { total++ }); err != nil {
			return fmt.Errorf("count view items: %w", err)
		}

		score, err := scoreReports(reports, include)
		if err != nil {
			return fmt.Errorf("calculate mutation score: %w", err)
		}

		slog.Info("Calculated mutation score", "score", score.Score(), "reportsCount", reports.Len(), "mutationsCount", total)
		w.progress.DisplayUpcomingTestsInfo(ctx, total)

		if err := rangeViewItems(reports, include, func(mutation m.Mutation, result m.Result) {
			w.progress.DisplayStartingTestInfo(ctx, mutation, 0)
			w.progress.DisplayCompletedTestInfo(ctx, mutation, result)
		}); err != nil {
			return fmt.Errorf("display view items: %w", err)
		}

		w.progress.DisplayMutationScore(ctx, score)

		thresholds, err := w.viewThresholds(ctx, args)
		if err != nil {
			return err
		}

		return thresholds.Err(score)
	})
}

// loadReports loads the reports in dir, explaining a missing directory.
func (w *workflow) loadReports(ctx context.Context, dir m.Path) (pkg.FileSpill[m.Report], error) {
	reports, err := w.reports.LoadSpillReports(ctx, dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, noReportsError(dir)
	}

	if err != nil {
		slog.Error("Failed to load reports", "error", err, "path", dir)
		return nil, fmt.Errorf("load reports: %w", err)
	}

	return reports, nil
}

func joinPaths(paths []m.Path) string {
	parts := make([]string, 0, len(paths))
	for _, p := range paths {
		parts = append(parts, string(p))
	}

	return strings.Join(parts, " ")
}

func noReportsError(dir m.Path) error {
	return WithExitCode(m.ExitNoReports, fmt.Errorf("no reports directory at %s; run `gooze run` first, or pass -o with the reports directory", dir))
}

// summarizeView counts the results of the reports matching paths per file and
// collects their survivors.
func summarizeView(reports pkg.FileSpill[m.Report], paths []m.Path) (ReportSummary, error) {
	files := map[string]*FileSummary{}

	var summary ReportSummary

	err := reports.Range(func(_ uint64, report m.Report) error {
		if !matchesPaths(report.Source, paths) {
			return nil
		}

		key := report.Source.Key()

		file, ok := files[key]
		if !ok {
			file = &FileSummary{Path: key}
			files[key] = file
		}

		counts := summarizeReport(report)
		file.Merge(counts)
		summary.Score.Merge(counts)
		summary.Survivors = append(summary.Survivors, reportSurvivors(report)...)

		return nil
	})
	if err != nil {
		return ReportSummary{}, err
	}

	for _, key := range slices.Sorted(maps.Keys(files)) {
		summary.Files = append(summary.Files, *files[key])
	}

	summary.Score.Files = summary.Files

	slices.SortStableFunc(summary.Survivors, func(a, b Survivor) int {
		return cmp.Or(cmp.Compare(a.Path, b.Path), cmp.Compare(a.Line, b.Line), cmp.Compare(a.Mutagen, b.Mutagen))
	})

	return summary, nil
}

// reportSurvivors returns the scored, not killed mutations of a report, located
// by the first change in its diff.
func reportSurvivors(report m.Report) []Survivor {
	var change rpt.Change

	if report.Diff != nil {
		if changes := rpt.DiffChanges(*report.Diff); len(changes) > 0 {
			change = changes[0]
		}
	}

	var survivors []Survivor

	for _, mutationType := range sortedResultTypes(report.Result) {
		for _, entry := range report.Result[mutationType] {
			// Equivalent mutants: Killed is the lowest status.
			if entry.Status == rpt.Killed || entry.Status == rpt.Error { //gooze:ignore comparison
				continue
			}

			survivors = append(survivors, Survivor{
				Path:       report.Source.Key(),
				Line:       change.Line,
				Mutagen:    mutationType.Name,
				Status:     entry.Status,
				Original:   strings.TrimSpace(change.Original),
				Mutated:    strings.TrimSpace(change.Mutated),
				MutationID: entry.MutationID,
			})
		}
	}

	return survivors
}

// matchesPaths reports whether source matches any of the patterns, which read
// like `gooze run` arguments: "./..." matches every file, "dir/..." files under
// dir, "dir" files directly in dir and "dir/file.go" that file. Relative
// patterns match the module-relative path stored in reports, absolute ones the
// absolute path. No patterns match every source.
func matchesPaths(source rpt.Source, patterns []m.Path) bool {
	if len(patterns) == 0 {
		return true
	}

	if source.Origin == nil {
		return false
	}

	return slices.ContainsFunc(patterns, func(pattern m.Path) bool {
		return matchesPattern(*source.Origin, string(pattern))
	})
}

func matchesPattern(origin rpt.File, pattern string) bool {
	target := origin.ShortPath
	if filepath.IsAbs(pattern) {
		target = origin.FullPath
	}

	if target == "" {
		return false
	}

	pattern = filepath.ToSlash(pattern)
	target = path.Clean(filepath.ToSlash(target))

	dir, recursive := strings.CutSuffix(pattern, "/...")
	if pattern == "..." {
		dir, recursive = ".", true
	}

	dir = path.Clean(dir)

	switch {
	case recursive:
		return dir == "." || target == dir || strings.HasPrefix(target, dir+"/")
	default:
		return target == dir || path.Dir(target) == dir
	}
}

// rangeViewItems calls fn with one mutation and its result for every entry in
// the reports include accepts, in a deterministic order, without collecting
// them.
func rangeViewItems(reports pkg.FileSpill[m.Report], include func(rpt.Source) bool, fn func(m.Mutation, m.Result)) error {
	return reports.Range(func(_ uint64, report m.Report) error {
		if !include(report.Source) {
			return nil
		}

		for _, mutationType := range sortedResultTypes(report.Result) {
			for _, entry := range report.Result[mutationType] {
				mutation := m.Mutation{ID: entry.MutationID, Source: report.Source, Type: mutationType}
				// Equivalent mutants: Killed is the lowest status.
				if entry.Status != rpt.Killed && report.Diff != nil { //gooze:ignore comparison
					mutation.DiffCode = *report.Diff
				}

				fn(mutation, m.Result{mutationType: {entry}})
			}
		}

		return nil
	})
}

// sortedResultTypes returns the mutation types in a result, ordered by name then
// version, so view output is deterministic.
func sortedResultTypes(result m.Result) []mgen.Type {
	return slices.SortedFunc(maps.Keys(result), func(a, b mgen.Type) int {
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.Version, b.Version))
	})
}
