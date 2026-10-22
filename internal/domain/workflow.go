package domain

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
	m "gooze.dev/pkg/gooze/internal/model"
	pkg "gooze.dev/pkg/gooze/pkg"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// DefaultMutations defines the default set of mutation types to generate.
var DefaultMutations = mgen.Defaults()

// ShardDirPrefix is the directory name prefix used when storing sharded reports.
const ShardDirPrefix = "shard_"

// EstimateArgs contains the arguments for estimating mutations.
type EstimateArgs struct {
	Paths    []m.Path
	Exclude  []string
	UseCache bool
	Reports  m.Path
	// Diff is a git ref; when set, only lines changed since its merge base
	// with HEAD are mutated. Empty mutates everything in scope.
	Diff string
	// Run is a regular expression, as for go test -run; when set, only
	// functions whose name or Type.Name matches are mutated.
	Run string
	// Mutagens names the mutagens to run (items may be comma-separated);
	// empty runs all of DefaultMutations.
	Mutagens []string
}

// TestArgs contains the arguments for running mutation tests.
type TestArgs struct {
	EstimateArgs
	Reports         m.Path
	Threads         int
	ShardIndex      int
	TotalShardCount int
	MutationTimeout time.Duration
	CoverageProfile m.Path
	// Workspace selects how mutants are built: through -overlay in the project
	// (the default) or in a per-worker deep copy of it.
	Workspace WorkspaceMode
	// Thresholds are the minimum scores the run must reach. A shard doesn't
	// check them: report merge does, on all the results.
	Thresholds Thresholds
}

// ViewArgs contains the arguments for viewing mutation test reports.
type ViewArgs struct {
	Reports m.Path
	// Paths limits the view to matching sources, see matchesPaths. Empty means
	// every source.
	Paths []m.Path
	// All lists every mutation with its status instead of a per-file summary.
	All bool
	// Thresholds are the minimum scores of the viewed reports.
	Thresholds Thresholds
	// DiffThresholds replace Thresholds when the reports are a --diff run's,
	// as they do for gooze run --diff.
	DiffThresholds Thresholds
	// Format selects how survivors are written to Output: as text in the
	// summary (the default), as GitHub annotations before it, or as SARIF
	// instead of it.
	Format ViewFormat
	Output io.Writer
	// PathPrefix is prepended to report paths in GitHub and SARIF output to
	// make them relative to the repository root.
	PathPrefix string
	// ToolVersion is gooze's version, recorded in SARIF output.
	ToolVersion string
}

// MergeArgs contains the arguments for merging sharded mutation test reports.
type MergeArgs struct {
	Reports m.Path
	// Thresholds are the minimum scores of the merged reports.
	Thresholds Thresholds
}

// Workflow defines the interface for the mutation testing workflow.
type Workflow interface {
	Estimate(ctx context.Context, args EstimateArgs) error
	Test(ctx context.Context, args TestArgs) error
	View(ctx context.Context, args ViewArgs) error
	Merge(ctx context.Context, args MergeArgs) error
}

type workflow struct {
	reports      ReportStore
	sources      SourceFSAdapter
	progress     Reporter
	orchestrator Orchestrator
	mutagen      Mutagen
	changes      ChangeDetector // nil disables diff mode
}

// NewWorkflow creates a new Workflow instance with the provided dependencies.
func NewWorkflow(
	fsAdapter SourceFSAdapter,
	reportStore ReportStore,
	reporter Reporter,
	orchestrator Orchestrator,
	mutagen Mutagen,
	opts ...WorkflowOption,
) Workflow {
	w := &workflow{
		sources:      fsAdapter,
		reports:      reportStore,
		progress:     reporter,
		orchestrator: orchestrator,
		mutagen:      mutagen,
	}

	for _, opt := range opts {
		opt(w)
	}

	return w
}

func (w *workflow) Estimate(ctx context.Context, args EstimateArgs) error {
	if err := w.progress.StartEstimate(ctx); err != nil {
		slog.Error("Failed to start workflow UI", "error", err)
		return err
	}

	estimation, err := w.estimateMutations(ctx, args)
	if err != nil {
		w.progress.Close(ctx)
		slog.Error("Failed to estimate mutations", "error", err)

		return fmt.Errorf("estimate mutations: %w", err)
	}

	if err := w.progress.DisplayEstimation(ctx, estimation, nil); err != nil {
		w.progress.Close(ctx)
		slog.Error("Failed to display estimation", "error", err)

		return fmt.Errorf("display: %w", err)
	}

	// Wait for UI to be closed by user (press 'q')
	w.progress.Wait(ctx)
	w.progress.Close(ctx)

	return nil
}

func (w *workflow) Test(ctx context.Context, args TestArgs) error {
	return w.withTestUI(ctx, func() error {
		slog.Info("Starting mutation testing", "threads", args.Threads, "shardIndex", args.ShardIndex, "totalShardCount", args.TotalShardCount)
		w.progress.DisplayConcurrencyInfo(ctx, args.Threads, args.ShardIndex, args.TotalShardCount)

		sources, sel, err := w.testSources(ctx, args.EstimateArgs)
		if err != nil {
			return err
		}

		defer sources.close()

		reportsDir, err := w.prepareReportsDir(ctx, args, sel, sources)
		if err != nil {
			return err
		}

		inThisShard := func(mutation m.Mutation) bool {
			return inShard(mutation.ID, args.ShardIndex, args.TotalShardCount)
		}

		mutations, total, err := w.generateMutations(ctx, sources.changed, inThisShard, sel)
		if err != nil {
			slog.Error("Failed to generate mutations", "error", err)
			return fmt.Errorf("generate mutations: %w", err)
		}

		defer closeSpill("mutations", mutations)

		w.progress.DisplayUpcomingTestsInfo(ctx, total)

		gate, err := w.loadCoverage(ctx, args.CoverageProfile)
		if err != nil {
			slog.Error("Failed to load coverage profile", "error", err)
			return fmt.Errorf("load coverage profile: %w", err)
		}

		reports, err := w.testReports(ctx, mutations, gate, sel, args.Threads, args.MutationTimeout, args.Workspace)
		if err != nil {
			slog.Error("Failed to run mutation tests", "error", err)
			return fmt.Errorf("run mutation tests: %w", err)
		}

		slog.Info("Completed mutation tests", "reportsCount", reports.Len())

		return w.finalizeReports(ctx, args, sel, reportsDir, sources, reports)
	})
}

// prepareReportsDir returns the directory the run writes to. A full run first
// drops the stored reports of deleted sources; a narrowed run leaves the main
// reports alone and starts its own directory afresh.
func (w *workflow) prepareReportsDir(ctx context.Context, args TestArgs, sel Selection, sources sourceSet) (m.Path, error) {
	reportsDir, err := runReportsDir(args, sel)
	if err != nil || sel.Narrowed() {
		return reportsDir, err
	}

	return reportsDir, w.cleanDeleted(ctx, args.Reports, reportsDir, sources.deleted)
}

func (w *workflow) cleanDeleted(ctx context.Context, base, reportsDir m.Path, deleted pkg.FileSpill[rpt.Source]) error {
	if deleted == nil || deleted.Len() == 0 {
		return nil
	}

	if err := w.reports.CleanReports(ctx, base, deleted); err != nil {
		slog.Error("Failed to clean reports for deleted sources", "error", err)
		return fmt.Errorf("clean reports: %w", err)
	}

	if base == reportsDir {
		return nil
	}

	return w.regenerateIndex(ctx, base)
}

// generateMutations generates every mutation of the given sources once,
// keeping those include accepts and sel selects, and spills them so the test
// pass can replay them without regenerating or holding them in memory.
func (w *workflow) generateMutations(ctx context.Context, sources pkg.FileSpill[rpt.Source], include func(m.Mutation) bool, sel Selection) (pkg.FileSpill[m.Mutation], int, error) {
	mutations, err := pkg.NewFileSpill[m.Mutation]()
	if err != nil {
		return nil, 0, fmt.Errorf("create mutations spill: %w", err)
	}

	total := 0

	err = sources.Range(func(_ uint64, source rpt.Source) error {
		return w.mutagen.StreamMutations(ctx, source, func(mutation m.Mutation) error {
			if (include != nil && !include(mutation)) || !sel.Includes(mutation) {
				return nil
			}

			total++

			return mutations.Append(mutation)
		}, sel.Types()...)
	})
	if err != nil {
		closeSpill("mutations", mutations)
		return nil, 0, err
	}

	return mutations, total, nil
}

func closeSpill[T any](name string, spill pkg.FileSpill[T]) {
	if spill == nil {
		return
	}

	if err := spill.Close(); err != nil {
		slog.Error("Failed to close spill", "spill", name, "error", err)
	}
}

// finalizeReports replaces previous reports for the tested sources with the
// spilled ones, regenerates the reports index and displays the run's score.
func (w *workflow) finalizeReports(ctx context.Context, args TestArgs, sel Selection, reportsDir m.Path, sources sourceSet, reports pkg.FileSpill[m.Report]) error {
	defer closeSpill("reports", reports)

	if err := w.reports.CleanReports(ctx, reportsDir, sources.changed); err != nil {
		slog.Error("Failed to clean stale reports", "error", err, "path", reportsDir)
		return fmt.Errorf("clean stale reports: %w", err)
	}

	if err := w.reports.SaveReports(ctx, reportsDir, reports); err != nil {
		slog.Error("Failed to save reports", "error", err, "path", reportsDir)
		return fmt.Errorf("save reports: %w", err)
	}

	slog.Debug("Saved reports", "path", reportsDir)

	if err := w.reports.RegenerateIndex(ctx, reportsDir); err != nil {
		slog.Error("Failed to regenerate reports index", "error", err, "path", reportsDir)
		return fmt.Errorf("regenerate index: %w", err)
	}

	slog.Debug("Regenerated reports index", "path", reportsDir)

	if sel.Narrowed() {
		if err := w.reports.SaveSelection(ctx, reportsDir, sel.record()); err != nil {
			slog.Error("Failed to save the run's selection", "error", err, "path", reportsDir)
			return fmt.Errorf("save selection: %w", err)
		}
	}

	score, err := w.runScore(ctx, args, sel, reportsDir, sources, reports)
	if err != nil {
		return fmt.Errorf("calculate mutation score: %w", err)
	}

	score.ThresholdsSkipped = args.Thresholds.Set() && args.TotalShardCount > 1

	slog.Info("Calculated mutation score", "score", score.Score(), "cached", score.Cached, "tested", score.Tested)
	w.progress.DisplayMutationScore(ctx, score)

	if score.ThresholdsSkipped {
		return nil
	}

	return args.Thresholds.Err(score)
}

// runScore scores a run over every source in its scope: the results tested now
// plus, for an unsharded cached run, the stored results of unchanged sources.
// A shard scores only its own results, and a narrowed run only its selection;
// `report merge` scores the whole set.
func (w *workflow) runScore(ctx context.Context, args TestArgs, sel Selection, reportsDir m.Path, sources sourceSet, tested pkg.FileSpill[m.Report]) (MutationScore, error) {
	score, err := scoreReports(tested, nil)
	if err != nil {
		return MutationScore{}, err
	}

	score.Tested = score.Total

	if sel.Narrowed() {
		score.Selection = sel.Describe()
		score.ReportsDir = reportsDir

		return score, nil
	}

	if !sources.cached || args.TotalShardCount > 1 {
		return score, nil
	}

	inScope, err := sourceKeys(sources.scanned)
	if err != nil {
		return MutationScore{}, err
	}

	stored, err := w.reports.LoadSpillReports(ctx, args.Reports)
	if err != nil {
		return MutationScore{}, fmt.Errorf("load reports: %w", err)
	}

	defer closeSpill("stored reports", stored)

	scope, err := scoreReports(stored, func(source rpt.Source) bool {
		_, ok := inScope[source.Key()]
		return ok
	})
	if err != nil {
		return MutationScore{}, err
	}

	scope.Cached = max(scope.Total-score.Tested, 0)
	scope.Tested = score.Tested

	return scope, nil
}

func shardReportsDir(base m.Path, shardIndex int, totalShardCount int) m.Path {
	if totalShardCount <= 1 {
		return base
	}

	return m.Path(filepath.Join(string(base), fmt.Sprintf("%s%d", ShardDirPrefix, shardIndex)))
}

func (w *workflow) Merge(ctx context.Context, args MergeArgs) error {
	base := args.Reports
	slog.Info("Merging sharded mutation test reports", "basePath", base)

	if string(base) == "" {
		slog.Error("Reports directory path is required but not provided")
		return fmt.Errorf("reports directory path is required")
	}

	return w.withTestUI(ctx, func() error {
		shardDirs, err := w.findShardDirs(base)
		if err != nil {
			slog.Error("Failed to find shard directories", "error", err, "basePath", base)
			return err
		}

		if err := w.mergeShards(ctx, base, shardDirs); err != nil {
			return err
		}

		if err := w.mergeRunReports(ctx, base); err != nil {
			slog.Error("Failed to merge the run reports", "error", err, "basePath", base)
			return err
		}

		return w.regenerateIndexAndScore(ctx, base, args.Thresholds)
	})
}

// mergeShards moves the shard reports into base and removes the shard dirs.
func (w *workflow) mergeShards(ctx context.Context, base m.Path, shardDirs []string) error {
	if err := w.mergeReports(ctx, base, shardDirs); err != nil {
		slog.Error("Failed to merge shard reports", "error", err, "basePath", base)
		return err
	}

	slog.Info("Merged shard reports", "basePath", base)

	return w.removeShardDirs(shardDirs)
}

func (w *workflow) regenerateIndexAndScore(ctx context.Context, base m.Path, thresholds Thresholds) error {
	if err := w.regenerateIndex(ctx, base); err != nil {
		return err
	}

	return w.displayScore(ctx, base, thresholds)
}

// displayScore shows the score of the reports at base and checks it against
// thresholds.
func (w *workflow) displayScore(ctx context.Context, base m.Path, thresholds Thresholds) error {
	reports, err := w.reports.LoadSpillReports(ctx, base)
	if err != nil {
		return fmt.Errorf("load reports from base: %w", err)
	}

	defer closeSpill("reports", reports)

	score, err := scoreReports(reports, nil)
	if err != nil {
		return fmt.Errorf("calculate mutation score: %w", err)
	}

	w.progress.DisplayMutationScore(ctx, score)

	return thresholds.Err(score)
}

func (w *workflow) findShardDirs(base m.Path) ([]string, error) {
	shardDirs, err := findShardDirs(string(base))
	if errors.Is(err, os.ErrNotExist) {
		return nil, noReportsError(base)
	}

	if err != nil {
		slog.Error("Failed to find shard directories", "error", err, "basePath", base)
		return nil, fmt.Errorf("find shard directories: %w", err)
	}

	return shardDirs, nil
}

func (w *workflow) regenerateIndex(ctx context.Context, base m.Path) error {
	if err := w.reports.RegenerateIndex(ctx, base); err != nil {
		return fmt.Errorf("regenerate index: %w", err)
	}

	return nil
}

// mergeReports moves the shard reports into base. Baseline reports for sources
// a shard re-tested are removed first, as are baseline reports for files that
// no longer exist when the merge runs inside the project checkout.
func (w *workflow) mergeReports(ctx context.Context, base m.Path, shardDirs []string) error {
	shardReports, err := w.loadShardReports(ctx, shardDirs)
	if err != nil {
		return err
	}

	defer closeSpill("shard reports", shardReports)

	stale, err := w.staleBaselineSources(ctx, base, shardReports)
	if err != nil {
		return err
	}

	defer closeSpill("stale sources", stale)

	if err := w.reports.CleanReports(ctx, base, stale); err != nil {
		return fmt.Errorf("clean superseded reports: %w", err)
	}

	if err := w.reports.SaveReports(ctx, base, shardReports); err != nil {
		slog.Error("Failed to save merged reports", "error", err, "basePath", base)
		return fmt.Errorf("save merged reports: %w", err)
	}

	slog.Debug("Merged reports from shards", "shards", shardReports.Len(), "superseded", stale.Len())

	return w.regenerateIndex(ctx, base)
}

func (w *workflow) loadShardReports(ctx context.Context, shardDirs []string) (pkg.FileSpill[m.Report], error) {
	merged, err := pkg.NewFileSpill[m.Report]()
	if err != nil {
		return nil, fmt.Errorf("create shard reports spill: %w", err)
	}

	for _, shardDir := range shardDirs {
		if err := w.appendReportsFrom(ctx, m.Path(shardDir), merged); err != nil {
			closeSpill("shard reports", merged)
			slog.Error("Failed to load shard reports", "error", err, "shardDir", shardDir)

			return nil, fmt.Errorf("load shard reports from %s: %w", shardDir, err)
		}
	}

	return merged, nil
}

func (w *workflow) appendReportsFrom(ctx context.Context, dir m.Path, into pkg.FileSpill[m.Report]) error {
	reports, err := w.reports.LoadSpillReports(ctx, dir)
	if err != nil {
		return err
	}

	defer closeSpill("reports", reports)

	return reports.Range(func(_ uint64, report m.Report) error {
		return into.Append(report)
	})
}

// staleBaselineSources returns the baseline sources to drop from base: every
// source a shard re-tested, plus, when the merge runs in the project checkout,
// baseline sources whose file no longer exists.
func (w *workflow) staleBaselineSources(ctx context.Context, base m.Path, shardReports pkg.FileSpill[m.Report]) (pkg.FileSpill[rpt.Source], error) {
	stale, err := pkg.NewFileSpill[rpt.Source]()
	if err != nil {
		return nil, fmt.Errorf("create stale sources spill: %w", err)
	}

	addStale := uniqueSourceAppender(stale)

	inCheckout, err := shardSourcesInCheckout(shardReports, addStale)
	if err == nil && inCheckout {
		err = w.rangeBaseline(ctx, base, func(report m.Report) error {
			if sourceFileExists(report.Source) {
				return nil
			}

			return addStale(report.Source)
		})
	}

	if err != nil {
		closeSpill("stale sources", stale)
		return nil, err
	}

	return stale, nil
}

func uniqueSourceAppender(into pkg.FileSpill[rpt.Source]) func(rpt.Source) error {
	seen := make(map[string]struct{})

	return func(source rpt.Source) error {
		key := source.Key()
		if key == "" {
			return nil
		}

		if _, ok := seen[key]; ok {
			return nil
		}

		seen[key] = struct{}{}

		return into.Append(source)
	}
}

// shardSourcesInCheckout passes every shard source to add and reports whether
// all shard sources with a project-relative path resolve from the current
// directory, i.e. the merge runs inside the project checkout.
func shardSourcesInCheckout(shardReports pkg.FileSpill[m.Report], add func(rpt.Source) error) (bool, error) {
	checked, resolvable := false, true

	err := shardReports.Range(func(_ uint64, report m.Report) error {
		if report.Source.Origin != nil && report.Source.Origin.ShortPath != "" {
			checked = true
			resolvable = resolvable && sourceFileExists(report.Source)
		}

		return add(report.Source)
	})

	return checked && resolvable, err
}

func (w *workflow) rangeBaseline(ctx context.Context, base m.Path, fn func(m.Report) error) error {
	baseline, err := w.reports.LoadSpillReports(ctx, base)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("load existing reports from base: %w", err)
	}

	defer closeSpill("baseline reports", baseline)

	return baseline.Range(func(_ uint64, report m.Report) error {
		return fn(report)
	})
}

func sourceFileExists(source rpt.Source) bool {
	if source.Origin == nil {
		return false
	}

	for _, candidate := range []string{source.Origin.ShortPath, source.Origin.FullPath} {
		if candidate == "" {
			continue
		}

		if _, err := os.Stat(candidate); err == nil {
			return true
		}
	}

	return false
}

func (w *workflow) removeShardDirs(shardDirs []string) error {
	for _, shardDir := range shardDirs {
		if err := os.RemoveAll(shardDir); err != nil {
			return fmt.Errorf("remove shard directory %s: %w", shardDir, err)
		}
	}

	return nil
}

func findShardDirs(baseDir string) ([]string, error) {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, os.ErrNotExist
		}

		return nil, err
	}

	shardDirs := make([]string, 0)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		if !strings.HasPrefix(entry.Name(), ShardDirPrefix) {
			continue
		}

		shardDirs = append(shardDirs, filepath.Join(baseDir, entry.Name()))
	}

	sort.Strings(shardDirs)

	return shardDirs, nil
}

func (w *workflow) withTestUI(ctx context.Context, fn func() error) error {
	slog.Info("Starting workflow in test mode")

	if err := w.progress.StartTest(ctx); err != nil {
		slog.Error("Failed to start workflow in test mode", "error", err)
		return err
	}

	defer func() {
		slog.Info("Closing workflow UI")
		w.progress.Close(ctx)
	}()

	err := fn()
	if err != nil {
		return err
	}

	// Wait for UI to be closed by user (press 'q')
	w.progress.Wait(ctx)

	return nil
}

// sourceSet holds the sources a run or an estimate operates on. Without
// caching every scanned source is changed and deleted is empty.
type sourceSet struct {
	scanned pkg.FileSpill[rpt.Source] // every source in the run's scope
	changed pkg.FileSpill[rpt.Source] // the sources to (re-)test
	deleted pkg.FileSpill[rpt.Source] // stored sources whose files are gone, or nil
	cached  bool                      // whether unchanged sources reuse stored reports
}

func (s sourceSet) close() {
	// changed may be scanned itself; closing a spill twice is a no-op.
	closeSpill("scanned sources", s.scanned)
	closeSpill("changed sources", s.changed)
	closeSpill("deleted sources", s.deleted)
}

// resolveSources scans the sources in scope. With caching enabled, unchanged
// files are left out of changed and sources whose files are gone are returned
// as deleted. A file also counts as changed when one of types generates
// mutations for it but its stored reports hold no results of that mutagen,
// such as a mutagen added to gooze after the file was tested. The caller must
// close the set.
func (w *workflow) resolveSources(ctx context.Context, args EstimateArgs, types []mgen.Type) (sourceSet, error) {
	scanned, err := w.scanSources(ctx, args)
	if err != nil {
		return sourceSet{}, err
	}

	if !args.UseCache || args.Reports == "" {
		return sourceSet{scanned: scanned, changed: scanned}, nil
	}

	changed, deleted, err := w.reports.CheckUpdates(ctx, args.Reports, scanned, w.untested(ctx, types))
	if err != nil {
		closeSpill("scanned sources", scanned)
		return sourceSet{}, fmt.Errorf("check updates: %w", err)
	}

	return sourceSet{scanned: scanned, changed: changed, deleted: deleted, cached: true}, nil
}

// errFound stops a mutation stream at the first mutation.
var errFound = errors.New("found a mutation")

// untested reports whether any of types that a source's stored reports hold
// no results of generates a mutation for it. A source that can't be read or
// parsed counts as untested, so the run reports the error.
func (w *workflow) untested(ctx context.Context, types []mgen.Type) func(rpt.Source, []string) bool {
	return func(source rpt.Source, tested []string) bool {
		missing := slices.DeleteFunc(slices.Clone(types), func(t mgen.Type) bool {
			return slices.Contains(tested, t.Name)
		})
		if len(missing) == 0 {
			return false
		}

		err := w.mutagen.StreamMutations(ctx, source, func(m.Mutation) error { return errFound }, missing...)

		return err != nil
	}
}

// sourceKeys returns the Key of every source in the spill.
func sourceKeys(sources pkg.FileSpill[rpt.Source]) (map[string]struct{}, error) {
	keys := make(map[string]struct{})

	err := sources.Range(func(_ uint64, source rpt.Source) error {
		keys[source.Key()] = struct{}{}
		return nil
	})

	return keys, err
}

// scanSources drains the streaming scanner into a spill.
func (w *workflow) scanSources(ctx context.Context, args EstimateArgs) (pkg.FileSpill[rpt.Source], error) {
	sourceCh, scanErrc := w.sources.Stream(ctx, args.Paths, args.Exclude...)

	sources, err := pkg.NewFileSpill[rpt.Source]()
	if err != nil {
		return nil, fmt.Errorf("create sources spill: %w", err)
	}

	var appendErr error

	for source := range sourceCh {
		if appendErr != nil {
			continue
		}

		appendErr = sources.Append(source)
	}

	scanErr := <-scanErrc

	if err := errors.Join(appendErr, scanErr); err != nil {
		closeSpill("sources", sources)
		return nil, fmt.Errorf("get sources: %w", err)
	}

	return sources, nil
}

// estimateMutations streams sources and mutations and returns per-file counts,
// without ever holding the sources or mutations in memory all at once. Files
// whose stored reports are still valid are listed too, marked as cached.
func (w *workflow) estimateMutations(ctx context.Context, args EstimateArgs) (Estimation, error) {
	sel, err := w.selection(ctx, args)
	if err != nil {
		return Estimation{}, err
	}

	sources, err := w.resolveSources(ctx, args, sel.Types())
	if err != nil {
		return Estimation{}, err
	}

	defer sources.close()

	isCached := func(rpt.Source) bool { return false }

	if sources.cached {
		changed, err := sourceKeys(sources.changed)
		if err != nil {
			return Estimation{}, err
		}

		isCached = func(source rpt.Source) bool {
			_, ok := changed[source.Key()]
			return !ok
		}
	}

	return w.countMutations(ctx, sources.scanned, isCached, sel)
}

// countMutations generates the mutations for the given sources and aggregates
// per-file counts. Mutations are processed one at a time and never retained.
// Only the sources and mutations sel selects are counted.
func (w *workflow) countMutations(ctx context.Context, sources pkg.FileSpill[rpt.Source], isCached func(rpt.Source) bool, sel Selection) (Estimation, error) {
	tally := newEstimateTally(sel)

	err := sources.Range(func(_ uint64, source rpt.Source) error {
		if !sel.InScope(source) {
			return nil
		}

		tally.addSource()

		cached := isCached(source)

		return w.mutagen.StreamMutations(ctx, source, func(mutation m.Mutation) error {
			if sel.Includes(mutation) {
				tally.add(mutation, cached)
			}

			return nil
		}, sel.Types()...)
	})
	if err != nil {
		return Estimation{}, err
	}

	return tally.estimation(), nil
}

// estimateTally accumulates per-file mutation counts for an Estimation.
type estimateTally struct {
	result Estimation
	byKey  map[string]*FileEstimate
	order  []string
}

func newEstimateTally(sel Selection) *estimateTally {
	tally := &estimateTally{byKey: map[string]*FileEstimate{}}

	if sel.Diff != nil {
		info := sel.Diff.Info()
		tally.result.Diff = &info
	}

	if sel.Run != nil {
		tally.result.Run = sel.Run.String()
	}

	for _, t := range sel.Mutagens {
		tally.result.Mutagens = append(tally.result.Mutagens, t.Name)
	}

	return tally
}

// addSource counts a source file in scope of a diff-mode estimate.
func (t *estimateTally) addSource() {
	if t.result.Diff != nil {
		t.result.Diff.Files++
	}
}

func (t *estimateTally) add(mutation m.Mutation, cached bool) {
	t.result.Total++

	if cached {
		t.result.Cached++
	}

	key, path := fileKey(mutation)

	estimate, ok := t.byKey[key]
	if !ok {
		estimate = &FileEstimate{Path: path, Cached: cached}
		t.byKey[key] = estimate
		t.order = append(t.order, key)
	}

	estimate.Count++
}

// estimation returns the counts, with files sorted by path.
func (t *estimateTally) estimation() Estimation {
	estimation := t.result
	estimation.Files = make([]FileEstimate, 0, len(t.order))

	for _, key := range t.order {
		estimation.Files = append(estimation.Files, *t.byKey[key])
	}

	sort.Slice(estimation.Files, func(i, j int) bool {
		files := estimation.Files
		if files[i].Path != files[j].Path {
			return files[i].Path < files[j].Path
		}

		return files[i].Count < files[j].Count
	})

	return estimation
}

func fileKey(mutation m.Mutation) (string, string) {
	if mutation.Source.Origin == nil {
		return "", ""
	}

	path := mutation.Source.Origin.ShortPath

	key := mutation.Source.Origin.Hash
	if key == "" {
		key = path
	}

	return key, path
}

// inShard reports whether a mutation ID is assigned to the given shard. A
// non-positive total means sharding is disabled and everything is included.
func inShard(id string, shardIndex int, totalShardCount int) bool {
	if totalShardCount <= 0 {
		return true
	}

	h := sha256.Sum256([]byte(id))

	hashValue := int(h[0])<<24 + int(h[1])<<16 + int(h[2])<<8 + int(h[3])
	if hashValue < 0 {
		hashValue = -hashValue
	}

	return hashValue%totalShardCount == shardIndex
}

// mutationOutcome is the result of testing one mutation, carried over the
// results channel from a worker to the collector.
type mutationOutcome struct {
	mutation m.Mutation
	result   m.Result
	err      error
}

// testReports runs mutation testing as a fan-out/fan-in channel pipeline:
//
//	dispatcher --> [ per-thread mutation queues ] --> workers --> results --> collector --> FileSpill
//
// The dispatcher streams mutations source-by-source and hands each one to a
// ready worker queue (one queue per configured thread). Each worker owns a
// reusable workspace, tests the mutation, and sends the outcome on the shared
// results channel. A single collector drains results and spills reports to disk.
// Peak memory is bounded by one file's mutations plus the small channel buffers,
// not the whole project's mutations.
// loadCoverage reads and parses the supplied coverage profile.
func (w *workflow) loadCoverage(ctx context.Context, profile m.Path) (*CoverageIndex, error) {
	if profile == "" {
		return ParseCoverage(nil)
	}

	content, err := w.sources.ReadFile(ctx, profile)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", profile, err)
	}

	return ParseCoverage(content)
}

func (w *workflow) testReports(
	ctx context.Context,
	mutations pkg.FileSpill[m.Mutation],
	gate *CoverageIndex,
	sel Selection,
	threads int,
	mutationTimeout time.Duration,
	workspace WorkspaceMode,
) (pkg.FileSpill[m.Report], error) {
	reports, err := pkg.NewFileSpill[m.Report]()
	if err != nil {
		slog.Error("failed to create reports filespill", "error", err)
		return nil, fmt.Errorf("create reports filespill: %w", err)
	}

	effectiveThreads := threads
	if effectiveThreads <= 0 {
		effectiveThreads = 1
	}

	// Unbuffered per-thread queues (count == --parallel): reflect.Select can only
	// hand a mutation to a thread whose worker is idle, giving true load balancing.
	queues := makeQueues(effectiveThreads)
	results := make(chan mutationOutcome, effectiveThreads)

	// Collector drains results into the spill while the dispatcher and workers run.
	collected := make(chan collectorResult, 1)

	go func() {
		collected <- w.collectResults(ctx, results, reports, sel.ReportScope)
	}()

	var group errgroup.Group

	group.Go(w.dispatchMutations(ctx, mutations, gate, queues, results))

	for threadID := range effectiveThreads {
		group.Go(w.consumeMutations(ctx, queues[threadID], threadID, mutationTimeout, workspace, results))
	}

	runErr := group.Wait()

	close(results)

	if err := testRunError(runErr, <-collected); err != nil {
		closeSpill("reports", reports)
		return nil, err
	}

	return reports, nil
}

func makeQueues(n int) []chan m.Mutation {
	queues := make([]chan m.Mutation, n)
	for i := range queues {
		queues[i] = make(chan m.Mutation)
	}

	return queues
}

// testRunError reduces the pipeline outcome to a single error: a fatal pipeline
// error wins, otherwise the aggregated per-mutation errors.
func testRunError(runErr error, outcome collectorResult) error {
	switch {
	case runErr != nil:
		return runErr
	case outcome.fatalErr != nil:
		return outcome.fatalErr
	case len(outcome.errs) > 0:
		return fmt.Errorf("errors occurred during mutation testing: %w", errors.Join(outcome.errs...))
	default:
		return nil
	}
}

// collectorResult aggregates everything the collector observed: non-fatal
// per-mutation errors and a fatal error (e.g. a spill write failure).
type collectorResult struct {
	errs     []error
	fatalErr error
}

// collectResults drains the results channel until it is closed, spilling a
// report for each successful outcome and accumulating errors. Because a single
// goroutine owns the report spill and the error slices, no locking is needed.
func (w *workflow) collectResults(
	ctx context.Context,
	results <-chan mutationOutcome,
	reports pkg.FileSpill[m.Report],
	scopeOf func(rpt.Source) *rpt.Scope,
) collectorResult {
	var collected collectorResult

	for outcome := range results {
		if outcome.err != nil {
			collected.errs = append(collected.errs, outcome.err)
			continue
		}

		if err := reports.Append(buildReport(outcome.mutation, outcome.result, scopeOf(outcome.mutation.Source))); err != nil {
			slog.Error("failed to append report to filespill", "error", err)

			if collected.fatalErr == nil {
				collected.fatalErr = fmt.Errorf("append report to filespill: %w", err)
			}

			continue
		}

		w.progress.DisplayCompletedTestInfo(ctx, outcome.mutation, outcome.result)
	}

	return collected
}

// buildReport records the result of mutation; scope is what the run's
// selection covered of the source (nil: all of it).
func buildReport(mutation m.Mutation, result m.Result, scope *rpt.Scope) m.Report {
	report := m.Report{
		Source: mutation.Source,
		Result: result,
		Line:   mutation.Line,
		Scope:  scope,
	}

	if getMutationStatus(result, mutation) != rpt.Killed {
		diff := mutation.DiffCode
		report.Diff = &diff
	}

	return report
}

// dispatchMutations returns a task that replays the generated mutations and
// hands each one to a ready worker queue, short-circuiting mutations on lines
// the coverage profile marks as uncovered, and closes every queue when done so
// the workers terminate.
func (w *workflow) dispatchMutations(
	ctx context.Context,
	mutations pkg.FileSpill[m.Mutation],
	gate *CoverageIndex,
	queues []chan m.Mutation,
	results chan<- mutationOutcome,
) func() error {
	return func() error {
		defer closeQueues(queues)

		return mutations.Range(func(_ uint64, mutation m.Mutation) error {
			if gate != nil && gate.Uncovered(mutation.Source.Origin.ShortPath, mutation.Line) {
				mutation.MutatedCode = nil

				return sendOutcome(ctx, results, mutationOutcome{
					mutation: mutation,
					result:   resultForStatus(mutation, rpt.NotCovered),
				})
			}

			return dispatch(ctx, queues, mutation)
		})
	}
}

// sendOutcome delivers a precomputed outcome to the collector, respecting cancellation.
func sendOutcome(ctx context.Context, results chan<- mutationOutcome, outcome mutationOutcome) error {
	select {
	case results <- outcome:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// dispatch sends a mutation to whichever worker queue can accept it first,
// blocking only when every queue is full. It returns the context error if the
// context is canceled while waiting.
func dispatch(ctx context.Context, queues []chan m.Mutation, mutation m.Mutation) error {
	cases := make([]reflect.SelectCase, 0, len(queues)+1)
	for _, queue := range queues {
		cases = append(cases, reflect.SelectCase{
			Dir:  reflect.SelectSend,
			Chan: reflect.ValueOf(queue),
			Send: reflect.ValueOf(mutation),
		})
	}

	cases = append(cases, reflect.SelectCase{
		Dir:  reflect.SelectRecv,
		Chan: reflect.ValueOf(ctx.Done()),
	})

	chosen, _, _ := reflect.Select(cases)
	if chosen == len(queues) {
		return ctx.Err()
	}

	return nil
}

func closeQueues(queues []chan m.Mutation) {
	for _, queue := range queues {
		close(queue)
	}
}

// consumeMutations returns a worker task that owns a single reusable workspace
// (so its setup happens once per worker, not once per mutation), tests each
// mutation arriving on its dedicated queue, and sends the outcome to the shared
// results channel.
func (w *workflow) consumeMutations(
	ctx context.Context,
	queue <-chan m.Mutation,
	threadID int,
	mutationTimeout time.Duration,
	workspace WorkspaceMode,
	results chan<- mutationOutcome,
) func() error {
	return func() error {
		ws := w.orchestrator.NewWorkspace(workspace)
		defer ws.Close(ctx)

		for mutation := range queue {
			outcome := w.runMutation(ctx, ws, mutation, threadID, mutationTimeout)

			select {
			case results <- outcome:
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		return nil
	}
}

// runMutation tests a single mutation under a per-mutation timeout and returns
// its outcome. The timeout starts here (at execution time), not when the
// mutation was queued.
func (w *workflow) runMutation(
	ctx context.Context,
	ws Workspace,
	mutation m.Mutation,
	threadID int,
	mutationTimeout time.Duration,
) mutationOutcome {
	w.progress.DisplayStartingTestInfo(ctx, mutation, threadID)

	mutationCtx := ctx

	var cancel context.CancelFunc
	if mutationTimeout > 0 {
		mutationCtx, cancel = context.WithTimeout(ctx, mutationTimeout)
	}

	result, err := ws.Run(mutationCtx, mutation)

	if cancel != nil {
		cancel()
	}

	mutation.MutatedCode = nil

	return mutationOutcome{mutation: mutation, result: result, err: err}
}

func getMutationStatus(result m.Result, mutation m.Mutation) rpt.TestStatus {
	entries, ok := result[mutation.Type]
	if !ok || len(entries) < 1 {
		return rpt.Error
	}

	for _, entry := range entries {
		if entry.MutationID == mutation.ID {
			return entry.Status
		}
	}

	// If the orchestrator returned entries for a different mutation ID, do not
	// guess: treating it as an error avoids inflating the score and attaching an
	// incorrect diff.
	return rpt.Error
}
