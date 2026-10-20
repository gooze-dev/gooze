package domain

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
	m "gooze.dev/pkg/gooze/internal/model"
	pkg "gooze.dev/pkg/gooze/pkg"
)

// DefaultMutations defines the default set of mutation types to generate.
var DefaultMutations = []m.MutationType{m.MutationArithmetic, m.MutationBoolean, m.MutationNumbers, m.MutationComparison, m.MutationLogical, m.MutationUnary}

// ShardDirPrefix is the directory name prefix used when storing sharded reports.
const ShardDirPrefix = "shard_"

// EstimateArgs contains the arguments for estimating mutations.
type EstimateArgs struct {
	Paths    []m.Path
	Exclude  []string
	UseCache bool
	Reports  m.Path
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
}

// ViewArgs contains the arguments for viewing mutation test reports.
type ViewArgs struct {
	Reports m.Path
}

// MergeArgs contains the arguments for merging sharded mutation test reports.
type MergeArgs struct {
	Reports m.Path
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
}

// NewWorkflow creates a new Workflow instance with the provided dependencies.
func NewWorkflow(
	fsAdapter SourceFSAdapter,
	reportStore ReportStore,
	reporter Reporter,
	orchestrator Orchestrator,
	mutagen Mutagen,
) Workflow {
	return &workflow{
		sources:      fsAdapter,
		reports:      reportStore,
		progress:     reporter,
		orchestrator: orchestrator,
		mutagen:      mutagen,
	}
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

		reportsDir := shardReportsDir(args.Reports, args.ShardIndex, args.TotalShardCount)

		sources, deleted, err := w.changedSources(ctx, args.EstimateArgs)
		if err != nil {
			slog.Error("Failed to resolve sources", "error", err)
			return err
		}

		defer closeSpill("sources", sources)
		defer closeSpill("deleted sources", deleted)

		if err := w.cleanDeleted(ctx, args.Reports, reportsDir, deleted); err != nil {
			return err
		}

		inThisShard := func(mutation m.Mutation) bool {
			return inShard(mutation.ID, args.ShardIndex, args.TotalShardCount)
		}

		mutations, total, err := w.generateMutations(ctx, sources, inThisShard)
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

		reports, err := w.testReports(ctx, mutations, gate, args.Threads, args.MutationTimeout)
		if err != nil {
			slog.Error("Failed to run mutation tests", "error", err)
			return fmt.Errorf("run mutation tests: %w", err)
		}

		slog.Info("Completed mutation tests", "reportsCount", reports.Len())

		return w.finalizeReports(ctx, reportsDir, sources, reports)
	})
}

func (w *workflow) cleanDeleted(ctx context.Context, base, reportsDir m.Path, deleted pkg.FileSpill[m.Source]) error {
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
// keeping those include accepts, and spills them so the test pass can replay
// them without regenerating or holding them in memory.
func (w *workflow) generateMutations(ctx context.Context, sources pkg.FileSpill[m.Source], include func(m.Mutation) bool) (pkg.FileSpill[m.Mutation], int, error) {
	mutations, err := pkg.NewFileSpill[m.Mutation]()
	if err != nil {
		return nil, 0, fmt.Errorf("create mutations spill: %w", err)
	}

	total := 0

	err = sources.Range(func(_ uint64, source m.Source) error {
		return w.mutagen.StreamMutations(ctx, source, func(mutation m.Mutation) error {
			if include != nil && !include(mutation) {
				return nil
			}

			total++

			return mutations.Append(mutation)
		}, DefaultMutations...)
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

// finalizeReports computes the mutation score, replaces previous reports for
// the tested sources with the spilled ones, and regenerates the reports index.
func (w *workflow) finalizeReports(ctx context.Context, reportsDir m.Path, tested pkg.FileSpill[m.Source], reports pkg.FileSpill[m.Report]) error {
	defer closeSpill("reports", reports)

	score, err := mutationScoreFromReports(reports)
	if err != nil {
		return fmt.Errorf("calculate mutation score: %w", err)
	}

	slog.Info("Calculated mutation score", "score", score)
	w.progress.DisplayMutationScore(ctx, score)

	if tested.Len() > 0 {
		if err := w.reports.CleanReports(ctx, reportsDir, tested); err != nil {
			slog.Error("Failed to clean stale reports", "error", err, "path", reportsDir)
			return fmt.Errorf("clean stale reports: %w", err)
		}
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

	return nil
}

func shardReportsDir(base m.Path, shardIndex int, totalShardCount int) m.Path {
	if totalShardCount <= 1 {
		return base
	}

	return m.Path(filepath.Join(string(base), fmt.Sprintf("%s%d", ShardDirPrefix, shardIndex)))
}

func (w *workflow) View(ctx context.Context, args ViewArgs) error {
	return w.withTestUI(ctx, func() error {
		slog.Info("Loading mutation test reports", "path", args.Reports)

		reports, err := w.reports.LoadSpillReports(ctx, args.Reports)
		if err != nil {
			slog.Error("Failed to load reports", "error", err, "path", args.Reports)
			return fmt.Errorf("load reports: %w", err)
		}

		defer closeSpill("reports", reports)

		total := 0

		if err := rangeViewItems(reports, func(m.Mutation, m.Result) { total++ }); err != nil {
			return fmt.Errorf("count view items: %w", err)
		}

		score, err := mutationScoreFromReports(reports)
		if err != nil {
			return fmt.Errorf("calculate mutation score: %w", err)
		}

		slog.Info("Calculated mutation score", "score", score, "reportsCount", reports.Len(), "mutationsCount", total)
		w.progress.DisplayUpcomingTestsInfo(ctx, total)

		if err := rangeViewItems(reports, func(mutation m.Mutation, result m.Result) {
			w.progress.DisplayStartingTestInfo(ctx, mutation, 0)
			w.progress.DisplayCompletedTestInfo(ctx, mutation, result)
		}); err != nil {
			return fmt.Errorf("display view items: %w", err)
		}

		w.progress.DisplayMutationScore(ctx, score)

		return nil
	})
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

		if len(shardDirs) == 0 {
			slog.Warn("No shard directories found; regenerating index only", "basePath", base)
			return w.regenerateIndexAndScore(ctx, base)
		}

		if err := w.mergeReports(ctx, base, shardDirs); err != nil {
			slog.Error("Failed to merge shard reports", "error", err, "basePath", base)
			return err
		}

		slog.Info("Merged shard reports", "basePath", base)

		if err := w.removeShardDirs(shardDirs); err != nil {
			return err
		}

		return w.displayScore(ctx, base)
	})
}

func (w *workflow) regenerateIndexAndScore(ctx context.Context, base m.Path) error {
	if err := w.regenerateIndex(ctx, base); err != nil {
		return err
	}

	return w.displayScore(ctx, base)
}

func (w *workflow) displayScore(ctx context.Context, base m.Path) error {
	reports, err := w.reports.LoadSpillReports(ctx, base)
	if errors.Is(err, os.ErrNotExist) {
		w.progress.DisplayMutationScore(ctx, mutationScore(0, 0))
		return nil
	}

	if err != nil {
		return fmt.Errorf("load reports from base: %w", err)
	}

	defer closeSpill("reports", reports)

	score, err := mutationScoreFromReports(reports)
	if err != nil {
		return fmt.Errorf("calculate mutation score: %w", err)
	}

	w.progress.DisplayMutationScore(ctx, score)

	return nil
}

func (w *workflow) findShardDirs(base m.Path) ([]string, error) {
	shardDirs, err := findShardDirs(string(base))
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

	if stale.Len() > 0 {
		if err := w.reports.CleanReports(ctx, base, stale); err != nil {
			return fmt.Errorf("clean superseded reports: %w", err)
		}
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
func (w *workflow) staleBaselineSources(ctx context.Context, base m.Path, shardReports pkg.FileSpill[m.Report]) (pkg.FileSpill[m.Source], error) {
	stale, err := pkg.NewFileSpill[m.Source]()
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

func uniqueSourceAppender(into pkg.FileSpill[m.Source]) func(m.Source) error {
	seen := make(map[string]struct{})

	return func(source m.Source) error {
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
func shardSourcesInCheckout(shardReports pkg.FileSpill[m.Report], add func(m.Source) error) (bool, error) {
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

func sourceFileExists(source m.Source) bool {
	if source.Origin == nil {
		return false
	}

	for _, candidate := range []m.Path{source.Origin.ShortPath, source.Origin.FullPath} {
		if candidate == "" {
			continue
		}

		if _, err := os.Stat(string(candidate)); err == nil {
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

// rangeViewItems calls fn with one mutation and its result for every entry in
// the reports, in a deterministic order, without collecting them.
func rangeViewItems(reports pkg.FileSpill[m.Report], fn func(m.Mutation, m.Result)) error {
	return reports.Range(func(_ uint64, report m.Report) error {
		for _, mutationType := range sortedResultTypes(report.Result) {
			for _, entry := range report.Result[mutationType] {
				mutation := m.Mutation{ID: entry.MutationID, Source: report.Source, Type: mutationType}
				if entry.Status != m.Killed && report.Diff != nil {
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
func sortedResultTypes(result m.Result) []m.MutationType {
	types := make([]m.MutationType, 0, len(result))
	for mutationType := range result {
		types = append(types, mutationType)
	}

	sort.Slice(types, func(i, j int) bool {
		if types[i].Name != types[j].Name {
			return types[i].Name < types[j].Name
		}

		return types[i].Version < types[j].Version
	})

	return types
}

// changedSources resolves the sources to operate on. With caching enabled,
// unchanged files are skipped and sources whose files are gone are returned
// as deleted. Both spills must be closed by the caller; deleted is nil when
// caching is off.
func (w *workflow) changedSources(ctx context.Context, args EstimateArgs) (pkg.FileSpill[m.Source], pkg.FileSpill[m.Source], error) {
	sources, err := w.scanSources(ctx, args)
	if err != nil {
		return nil, nil, err
	}

	if !args.UseCache || args.Reports == "" {
		return sources, nil, nil
	}

	defer closeSpill("scanned sources", sources)

	changed, deleted, err := w.reports.CheckUpdates(ctx, args.Reports, sources)
	if err != nil {
		return nil, nil, fmt.Errorf("check updates: %w", err)
	}

	return changed, deleted, nil
}

// scanSources drains the streaming scanner into a spill.
func (w *workflow) scanSources(ctx context.Context, args EstimateArgs) (pkg.FileSpill[m.Source], error) {
	sourceCh, scanErrc := w.sources.Stream(ctx, args.Paths, args.Exclude...)

	sources, err := pkg.NewFileSpill[m.Source]()
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
// without ever holding the sources or mutations in memory all at once.
func (w *workflow) estimateMutations(ctx context.Context, args EstimateArgs) (Estimation, error) {
	sources, deleted, err := w.changedSources(ctx, args)
	if err != nil {
		return Estimation{}, err
	}

	defer closeSpill("sources", sources)
	defer closeSpill("deleted sources", deleted)

	return w.countMutations(ctx, sources)
}

// countMutations generates the mutations for the given sources and aggregates
// per-file counts. Mutations are processed one at a time and never retained.
func (w *workflow) countMutations(ctx context.Context, sources pkg.FileSpill[m.Source]) (Estimation, error) {
	byKey := map[string]*FileEstimate{}
	order := make([]string, 0)
	total := 0

	err := sources.Range(func(_ uint64, source m.Source) error {
		return w.mutagen.StreamMutations(ctx, source, func(mutation m.Mutation) error {
			total++

			key, path := fileKey(mutation)

			estimate, ok := byKey[key]
			if !ok {
				estimate = &FileEstimate{Path: path}
				byKey[key] = estimate
				order = append(order, key)
			}

			estimate.Count++

			return nil
		}, DefaultMutations...)
	})
	if err != nil {
		return Estimation{}, err
	}

	files := make([]FileEstimate, 0, len(order))
	for _, key := range order {
		files = append(files, *byKey[key])
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].Path != files[j].Path {
			return files[i].Path < files[j].Path
		}

		return files[i].Count < files[j].Count
	})

	return Estimation{Total: total, Files: files}, nil
}

func fileKey(mutation m.Mutation) (string, string) {
	if mutation.Source.Origin == nil {
		return "", ""
	}

	path := string(mutation.Source.Origin.ShortPath)

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
	threads int,
	mutationTimeout time.Duration,
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
		collected <- w.collectResults(ctx, results, reports)
	}()

	var group errgroup.Group

	group.Go(w.dispatchMutations(ctx, mutations, gate, queues, results))

	for threadID := range effectiveThreads {
		group.Go(w.consumeMutations(ctx, queues[threadID], threadID, mutationTimeout, results))
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
		return fmt.Errorf("errors occurred during mutation testing: %v", outcome.errs)
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
) collectorResult {
	var collected collectorResult

	for outcome := range results {
		if outcome.err != nil {
			collected.errs = append(collected.errs, outcome.err)
			continue
		}

		if err := reports.Append(buildReport(outcome.mutation, outcome.result)); err != nil {
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

func buildReport(mutation m.Mutation, result m.Result) m.Report {
	report := m.Report{
		Source: mutation.Source,
		Result: result,
	}

	if getMutationStatus(result, mutation) != m.Killed {
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
			if gate != nil && gate.Uncovered(string(mutation.Source.Origin.ShortPath), mutation.Line) {
				mutation.MutatedCode = nil

				return sendOutcome(ctx, results, mutationOutcome{
					mutation: mutation,
					result:   resultForStatus(mutation, m.NotCovered),
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
// (so the project is copied once per worker, not once per mutation), tests each
// mutation arriving on its dedicated queue, and sends the outcome to the shared
// results channel.
func (w *workflow) consumeMutations(
	ctx context.Context,
	queue <-chan m.Mutation,
	threadID int,
	mutationTimeout time.Duration,
	results chan<- mutationOutcome,
) func() error {
	return func() error {
		ws := w.orchestrator.NewWorkspace()
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

func getMutationStatus(result m.Result, mutation m.Mutation) m.TestStatus {
	entries, ok := result[mutation.Type]
	if !ok || len(entries) < 1 {
		return m.Error
	}

	for _, entry := range entries {
		if entry.MutationID == mutation.ID {
			return entry.Status
		}
	}

	// If the orchestrator returned entries for a different mutation ID, do not
	// guess: treating it as an error avoids inflating the score and attaching an
	// incorrect diff.
	return m.Error
}
