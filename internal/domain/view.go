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
	// SurvivorLines holds Survivors per line when the view asks for it, else nil.
	SurvivorLines []SurvivorLine
	// BuildErrors are the mutations that did not build; they don't count
	// against the score. Sorted like Survivors.
	BuildErrors []Survivor
	Hint        string // what to do when the reports hold nothing, or ""
	// Brief leaves the survivors and build errors out of the text summary:
	// the per-file table and the score only.
	Brief bool
	// HiddenFiles counts the files a top-N view leaves out of Files; the
	// score still counts them.
	HiddenFiles int
	// Sections wraps the long parts of the text summary in collapsible CI
	// log sections (GitLab's section_start / section_end).
	Sections bool
	// Scope names the summary's scope when a merge shows two; zero otherwise.
	Scope ReportScope
}

// ReportScope names one of the scopes gooze report merge --scope diff,total
// shows: Name keeps their CI log sections apart, Title heads their table, and
// Selection, when set, names them in the score line.
type ReportScope struct {
	Name      string
	Title     string
	Selection string
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

	return w.showReports(ctx, args)
}

// showReports loads the reports once, writes args.Writes, shows them in
// args.Format and checks them against the thresholds.
func (w *workflow) showReports(ctx context.Context, args ViewArgs) error {
	_, breach, err := w.showScope(ctx, args)

	return cmp.Or(err, breach)
}

// showScope is showReports for one scope: it returns the score of the
// reports, the threshold breach, if any, and any other error apart.
func (w *workflow) showScope(ctx context.Context, args ViewArgs) (MutationScore, error, error) {
	slog.Info("Summarizing mutation test reports", "path", args.Reports, "paths", args.Paths)

	reports, err := w.loadReports(ctx, args.Reports)
	if err != nil {
		return MutationScore{}, nil, err
	}

	defer closeSpill("reports", reports)

	selection, err := w.reports.LoadSelection(ctx, args.Reports)
	if err != nil {
		return MutationScore{}, nil, fmt.Errorf("load the run's selection: %w", err)
	}

	summary, err := summarizeScope(reports, args, selection)
	if err != nil {
		return MutationScore{}, nil, err
	}

	scope := viewScope{args: args, diff: selection.Diff != ""}

	if err := w.writeViews(ctx, scope, summary); err != nil {
		return MutationScore{}, nil, err
	}

	shapeSummary(&summary, args)

	if err := w.displayView(ctx, scope, summary); err != nil {
		return MutationScore{}, nil, err
	}

	return summary.Score, scope.thresholds().Err(summary.Score), nil
}

// summarizeScope summarizes the reports of a scope: per file, with the
// changed lines named in the score of a --diff run's reports, survivors per
// line when asked for, and a hint when there are none.
func summarizeScope(reports pkg.FileSpill[m.Report], args ViewArgs, selection rpt.Selection) (ReportSummary, error) {
	summary, err := summarizeView(reports, args.Paths)
	if err != nil {
		return ReportSummary{}, fmt.Errorf("summarize reports: %w", err)
	}

	if len(summary.Files) == 0 && len(args.Paths) > 0 {
		return ReportSummary{}, WithExitCode(m.ExitNoReports, fmt.Errorf("no reports in %s match %s", args.Reports, joinPaths(args.Paths)))
	}

	if len(summary.Files) == 0 {
		summary.Hint = unmergedReportsHint(args.Reports)
	}

	if selection.Diff != "" {
		summary.Score.Selection = "changed lines since " + selection.Diff
	}

	summary.Scope = args.Scope
	summary.Score = summary.Score.timeoutsKilled(args.Thresholds.TimeoutsKilled)
	summary.Files = summary.Score.Files
	summary.Score.Selection = cmp.Or(args.Scope.Selection, summary.Score.Selection)

	if args.ByLine {
		summary.SurvivorLines = groupByLine(summary.Survivors)
	}

	return summary, nil
}

// viewScope is a view of the reports of one scope: the changed lines of a
// --diff run (diff) or the whole project.
type viewScope struct {
	args ViewArgs
	diff bool
}

// thresholds returns the thresholds to check the scope against: the diff
// ones for the changed lines, the plain ones otherwise, and none when the
// gate leaves the scope out.
func (s viewScope) thresholds() Thresholds {
	switch {
	case !s.args.Gate.checks(s.diff):
		return Thresholds{}
	case s.diff:
		return s.args.DiffThresholds
	default:
		return s.args.Thresholds
	}
}

// unmergedReportsHint points at the reports in dir that report view doesn't
// read: those of shards and of a narrowed run, until gooze report merge. It
// returns "" when there are none.
func unmergedReportsHint(dir m.Path) string {
	shards, _ := findShardDirs(string(dir))
	if len(shards) > 0 {
		names := make([]string, len(shards))
		for i, shard := range shards {
			names[i] = filepath.Base(shard)
		}

		return fmt.Sprintf("found %s but no merged reports; run: gooze report merge -o %s", strings.Join(names, ", "), dir)
	}

	runDir := filepath.Join(string(dir), RunDirName)
	if _, err := os.Stat(runDir); err != nil {
		return ""
	}

	return "the last --diff or --run reports are not merged; see them with: gooze report view -o " + runDir
}

// viewThresholds returns the thresholds to check the viewed reports against
// (see viewScope.thresholds).
func (w *workflow) viewThresholds(ctx context.Context, args ViewArgs) (Thresholds, error) {
	selection, err := w.reports.LoadSelection(ctx, args.Reports)
	if err != nil {
		return Thresholds{}, fmt.Errorf("load the run's selection: %w", err)
	}

	return viewScope{args: args, diff: selection.Diff != ""}.thresholds(), nil
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

		score = score.timeoutsKilled(args.Thresholds.TimeoutsKilled)

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

		survivors, buildErrors := reportFailures(report)
		summary.Survivors = append(summary.Survivors, survivors...)
		summary.BuildErrors = append(summary.BuildErrors, buildErrors...)

		return nil
	})
	if err != nil {
		return ReportSummary{}, err
	}

	for _, key := range slices.Sorted(maps.Keys(files)) {
		summary.Files = append(summary.Files, *files[key])
	}

	summary.Score.Files = summary.Files

	sortByLocation(summary.Survivors)
	sortByLocation(summary.BuildErrors)

	return summary, nil
}

// writeViews writes summary to each of args.Writes.
func (w *workflow) writeViews(ctx context.Context, scope viewScope, summary ReportSummary) error {
	for _, write := range scope.args.Writes {
		if err := w.writeView(ctx, scope, write, summary); err != nil {
			return err
		}
	}

	return nil
}

// shapeSummary applies the view's output settings to summary: a brief
// summary, the top files and collapsible sections.
func shapeSummary(summary *ReportSummary, args ViewArgs) {
	summary.Brief, summary.Sections = args.Summary, args.Sections
	if args.Top > 0 && args.Top < len(summary.Files) {
		summary.HiddenFiles = len(summary.Files) - args.Top
		summary.Files = topFiles(summary.Files, args.Top)
	}
}

// topFiles returns the n files of files with the most mutations that count
// against the score (survived, not covered, timed out or skipped), ties by
// path. files keeps its order.
func topFiles(files []FileSummary, n int) []FileSummary {
	against := func(file FileSummary) int {
		return file.Survived + file.NotCovered + file.Timeout + file.Skipped
	}

	ranked := slices.SortedStableFunc(slices.Values(files), func(a, b FileSummary) int {
		return cmp.Or(cmp.Compare(against(b), against(a)), cmp.Compare(a.Path, b.Path))
	})

	return ranked[:n]
}

func sortByLocation(survivors []Survivor) {
	slices.SortStableFunc(survivors, func(a, b Survivor) int {
		return cmp.Or(cmp.Compare(a.Path, b.Path), cmp.Compare(a.Line, b.Line), cmp.Compare(a.Mutagen, b.Mutagen))
	})
}

// reportFailures returns the not killed mutations of a report, located by the
// first change in its diff: the scored ones, and those that did not build.
func reportFailures(report m.Report) (survivors, buildErrors []Survivor) {
	var change rpt.Change

	if report.Diff != nil {
		if changes := rpt.DiffChanges(*report.Diff); len(changes) > 0 {
			change = changes[0]
		}
	}

	for _, mutationType := range sortedResultTypes(report.Result) {
		for _, entry := range report.Result[mutationType] {
			// Equivalent mutant: Killed is the lowest status.
			if entry.Status == rpt.Killed { //gooze:ignore comparison
				continue
			}

			failure := Survivor{
				Path:       report.Source.Key(),
				Line:       change.Line,
				Mutagen:    mutationType.Name,
				Status:     entry.Status,
				Original:   strings.TrimSpace(change.Original),
				Mutated:    strings.TrimSpace(change.Mutated),
				MutationID: entry.MutationID,
			}

			if entry.Status == rpt.Error {
				buildErrors = append(buildErrors, failure)
			} else {
				survivors = append(survivors, failure)
			}
		}
	}

	return survivors, buildErrors
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
