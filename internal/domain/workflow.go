package domain

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"gooze.dev/pkg/gooze/internal/adapter"
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
	reports      adapter.ReportStore
	sources      adapter.SourceFSAdapter
	progress     Reporter
	orchestrator Orchestrator
	mutagen      Mutagen
}

// NewWorkflow creates a new Workflow instance with the provided dependencies.
func NewWorkflow(
	fsAdapter adapter.SourceFSAdapter,
	reportStore adapter.ReportStore,
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
		slog.Debug("Using reports directory", "path", reportsDir)

		sources, err := w.changedSources(ctx, args.EstimateArgs)
		if err != nil {
			slog.Error("Failed to resolve sources", "error", err)
			return fmt.Errorf("generate mutations: %w", err)
		}

		inThisShard := func(mutation m.Mutation) bool {
			return inShard(mutation.ID, args.ShardIndex, args.TotalShardCount)
		}

		// Count-only streaming pass to size the progress display without
		// retaining any mutations in memory.
		estimation, err := w.countMutations(ctx, sources, inThisShard)
		if err != nil {
			slog.Error("Failed to count mutations", "error", err)
			return fmt.Errorf("generate mutations: %w", err)
		}

		slog.Debug("Counted mutations", "count", estimation.Total)
		w.progress.DisplayUpcomingTestsInfo(ctx, estimation.Total)

		reports, err := w.testReports(ctx, sources, inThisShard, args.Threads, args.MutationTimeout)
		if err != nil {
			slog.Error("Failed to run mutation tests", "error", err)
			return fmt.Errorf("run mutation tests: %w", err)
		}

		slog.Info("Completed mutation tests", "reportsCount", reports.Len())

		return w.finalizeReports(ctx, reportsDir, reports)
	})
}

// finalizeReports computes the mutation score, persists the spilled reports, and
// regenerates the reports index.
func (w *workflow) finalizeReports(ctx context.Context, reportsDir m.Path, reports pkg.FileSpill[m.Report]) error {
	score, err := mutationScoreFromReports(reports)
	if err != nil {
		return fmt.Errorf("calculate mutation score: %w", err)
	}

	slog.Info("Calculated mutation score", "score", score)
	w.progress.DisplayMutationScore(ctx, score)

	if err := w.reports.SaveSpillReports(ctx, reportsDir, reports); err != nil {
		slog.Error("Failed to save reports", "error", err, "path", reportsDir)
		return fmt.Errorf("save reports: %w", err)
	}

	if err := reports.Close(); err != nil {
		slog.Error("Failed to close reports spill", "error", err, "path", reportsDir)
		return fmt.Errorf("close reports spill: %w", err)
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
		slog.Debug("Using unsharded reports directory", "path", base)
		return base
	}

	slog.Debug("Using sharded reports directory", "base", base, "shardIndex", shardIndex)

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

		mutations, results, err := viewItemsFromReports(reports)
		if err != nil {
			return fmt.Errorf("extract view items from reports: %w", err)
		}

		slog.Debug("Loaded reports", "reportsCount", reports.Len(), "mutationsCount", len(mutations))

		score, err := mutationScoreFromReports(reports)
		if err != nil {
			return fmt.Errorf("calculate mutation score: %w", err)
		}

		slog.Info("Calculated mutation score", "score", score)
		w.progress.DisplayUpcomingTestsInfo(ctx, len(mutations))

		for i, mutation := range mutations {
			w.progress.DisplayStartingTestInfo(ctx, mutation, 0)
			w.progress.DisplayCompletedTestInfo(ctx, mutation, results[i])
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

	shardDirs, err := w.findShardDirs(base)
	if err != nil {
		slog.Error("Failed to find shard directories", "error", err, "basePath", base)
		return err
	}

	if len(shardDirs) == 0 {
		slog.Debug("No shard directories found; regenerating index only", "basePath", base)
		return w.regenerateIndex(ctx, base)
	}

	merged, err := w.mergeReports(ctx, base, shardDirs)
	if err != nil {
		slog.Error("Failed to merge shard reports", "error", err, "basePath", base)
		return err
	}

	slog.Info("Merged shard reports", "basePath", base)

	if err := w.saveMergedReports(ctx, base, merged); err != nil {
		return err
	}

	slog.Debug("Saved merged reports and regenerated index", "basePath", base)

	return w.removeShardDirs(shardDirs)
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

func (w *workflow) mergeReports(ctx context.Context, base m.Path, shardDirs []string) ([]m.Report, error) {
	merged := make([]m.Report, 0)

	// First, load existing reports from base directory to preserve cache.
	existingReports, err := w.loadReportsIfExists(ctx, base)
	if err != nil {
		return nil, fmt.Errorf("load existing reports from base: %w", err)
	}

	merged = append(merged, existingReports...)

	// Then load and merge reports from all shards.
	for _, shardDir := range shardDirs {
		reports, err := w.reports.LoadReports(ctx, m.Path(shardDir))
		if err != nil {
			slog.Error("Failed to load shard reports", "error", err, "shardDir", shardDir)
			return nil, fmt.Errorf("load shard reports from %s: %w", shardDir, err)
		}

		merged = append(merged, reports...)
	}

	slog.Debug("Merged reports from shards", "totalReports", len(merged))

	return merged, nil
}

func (w *workflow) loadReportsIfExists(ctx context.Context, path m.Path) ([]m.Report, error) {
	reports, err := w.reports.LoadReports(ctx, path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}

		return nil, err
	}

	return reports, nil
}

func (w *workflow) saveMergedReports(ctx context.Context, base m.Path, reports []m.Report) error {
	if err := w.reports.SaveReports(ctx, base, reports); err != nil {
		slog.Error("Failed to save merged reports", "error", err, "basePath", base)
		return fmt.Errorf("save merged reports: %w", err)
	}

	slog.Debug("Saved merged reports and regenerated index", "basePath", base)

	return w.regenerateIndex(ctx, base)
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

func viewItemsFromReports(reports pkg.FileSpill[m.Report]) ([]m.Mutation, []m.Result, error) {
	mutations := make([]m.Mutation, 0)
	results := make([]m.Result, 0)

	err := reports.Range(func(_ uint64, report m.Report) error {
		if len(report.Result) == 0 {
			return nil
		}

		mutationTypes := make([]m.MutationType, 0, len(report.Result))
		for mutationType := range report.Result {
			mutationTypes = append(mutationTypes, mutationType)
		}

		sort.Slice(mutationTypes, func(i, j int) bool {
			if mutationTypes[i].Name != mutationTypes[j].Name {
				return mutationTypes[i].Name < mutationTypes[j].Name
			}

			return mutationTypes[i].Version < mutationTypes[j].Version
		})

		for _, mutationType := range mutationTypes {
			entries := report.Result[mutationType]
			for _, entry := range entries {
				mutation := m.Mutation{
					ID:     entry.MutationID,
					Source: report.Source,
					Type:   mutationType,
				}
				if entry.Status == m.Survived && report.Diff != nil {
					mutation.DiffCode = *report.Diff
				}

				result := m.Result{}
				result[mutationType] = []struct {
					MutationID string
					Status     m.TestStatus
					Err        error
				}{
					{
						MutationID: entry.MutationID,
						Status:     entry.Status,
						Err:        entry.Err,
					},
				}

				mutations = append(mutations, mutation)
				results = append(results, result)
			}
		}

		return nil
	})
	if err != nil {
		slog.Error("Failed to extract view items from reports", "error", err)
		return nil, nil, err
	}

	return mutations, results, nil
}

// changedSources resolves the source files to operate on, applying incremental
// caching (skipping unchanged files and cleaning reports for deleted ones) when
// enabled.
func (w *workflow) changedSources(ctx context.Context, args EstimateArgs) ([]m.Source, error) {
	sources, err := w.sources.Get(ctx, args.Paths, args.Exclude...)
	if err != nil {
		return nil, fmt.Errorf("get sources: %w", err)
	}

	if !args.UseCache || args.Reports == "" {
		return sources, nil
	}

	changed, err := w.reports.CheckUpdates(ctx, args.Reports, sources)
	if err != nil {
		return nil, fmt.Errorf("check updates: %w", err)
	}

	currentByPath := w.buildSourcePathMap(sources)
	deleted, changedExisting := w.separateDeletedAndChanged(changed, currentByPath)

	if len(deleted) > 0 {
		if err := w.reports.CleanReports(ctx, args.Reports, deleted); err != nil {
			return nil, fmt.Errorf("clean reports: %w", err)
		}
	}

	return changedExisting, nil
}

func (w *workflow) buildSourcePathMap(sources []m.Source) map[string]m.Source {
	currentByPath := map[string]m.Source{}

	for _, src := range sources {
		if src.Origin != nil && src.Origin.FullPath != "" {
			currentByPath[string(src.Origin.FullPath)] = src
		}
	}

	return currentByPath
}

func (w *workflow) separateDeletedAndChanged(changed []m.Source, currentByPath map[string]m.Source) ([]m.Source, []m.Source) {
	deleted := make([]m.Source, 0)
	changedExisting := make([]m.Source, 0)

	for _, src := range changed {
		if src.Origin == nil || src.Origin.FullPath == "" {
			continue
		}

		if current, ok := currentByPath[string(src.Origin.FullPath)]; ok {
			changedExisting = append(changedExisting, current)
		} else {
			deleted = append(deleted, src)
		}
	}

	return deleted, changedExisting
}

// estimateMutations streams mutations for the given args and returns per-file
// counts, without ever holding the mutations themselves in memory.
func (w *workflow) estimateMutations(ctx context.Context, args EstimateArgs) (Estimation, error) {
	sources, err := w.changedSources(ctx, args)
	if err != nil {
		return Estimation{}, err
	}

	return w.countMutations(ctx, sources, nil)
}

// countMutations streams mutations for the given sources and aggregates per-file
// counts. include, when non-nil, filters which mutations are counted. Mutations
// are processed one at a time and never retained.
func (w *workflow) countMutations(ctx context.Context, sources []m.Source, include func(m.Mutation) bool) (Estimation, error) {
	byKey := map[string]*FileEstimate{}
	order := make([]string, 0)
	total := 0

	for _, source := range sources {
		err := w.mutagen.StreamMutations(ctx, source, func(mutation m.Mutation) error {
			if include != nil && !include(mutation) {
				return nil
			}

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
		if err != nil {
			return Estimation{}, err
		}
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

// ShardMutations filters mutations down to those that belong to the given shard.
// It is retained as a pure helper; the test pipeline shards inline via inShard.
func (w *workflow) ShardMutations(allMutations []m.Mutation, shardIndex int, totalShardCount int) []m.Mutation {
	if totalShardCount <= 0 {
		return allMutations
	}

	var shardMutations []m.Mutation

	for _, mutation := range allMutations {
		if inShard(mutation.ID, shardIndex, totalShardCount) {
			shardMutations = append(shardMutations, mutation)
		}
	}

	return shardMutations
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

// testReports runs mutation testing as a producer/worker pipeline. A single
// producer streams mutations source-by-source into a bounded channel; a pool of
// workers consumes them, runs the test, and spills each report to disk. Peak
// memory is bounded by one file's mutations plus the channel buffer, rather than
// the whole project's mutations.
func (w *workflow) testReports(
	ctx context.Context,
	sources []m.Source,
	include func(m.Mutation) bool,
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

	var (
		errsMu sync.Mutex
		errs   []error
		group  errgroup.Group
	)

	mutations := make(chan m.Mutation, effectiveThreads*2)

	// Producer streams mutations into the channel; workers consume and spill.
	group.Go(w.produceMutations(ctx, sources, include, mutations))

	for threadID := range effectiveThreads {
		group.Go(w.consumeMutations(ctx, mutations, threadID, mutationTimeout, reports, &errsMu, &errs))
	}

	if err := group.Wait(); err != nil {
		return reports, err
	}

	if len(errs) > 0 {
		return reports, fmt.Errorf("errors occurred during mutation testing: %v", errs)
	}

	return reports, nil
}

// produceMutations returns a task that streams mutations source-by-source into
// out, sharding inline, and closes out when done.
func (w *workflow) produceMutations(
	ctx context.Context,
	sources []m.Source,
	include func(m.Mutation) bool,
	out chan<- m.Mutation,
) func() error {
	return func() error {
		defer close(out)

		for _, source := range sources {
			err := w.mutagen.StreamMutations(ctx, source, func(mutation m.Mutation) error {
				if include != nil && !include(mutation) {
					return nil
				}

				select {
				case out <- mutation:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			}, DefaultMutations...)
			if err != nil {
				return err
			}
		}

		return nil
	}
}

// consumeMutations returns a worker task that tests each mutation from in.
func (w *workflow) consumeMutations(
	ctx context.Context,
	in <-chan m.Mutation,
	threadID int,
	mutationTimeout time.Duration,
	reports pkg.FileSpill[m.Report],
	errsMu *sync.Mutex,
	errs *[]error,
) func() error {
	return func() error {
		for mutation := range in {
			if err := w.runMutation(ctx, mutation, threadID, mutationTimeout, reports, errsMu, errs); err != nil {
				return err
			}
		}

		return nil
	}
}

// runMutation tests a single mutation under a per-mutation timeout and spills its
// report. The timeout starts here (at execution time), not when the mutation was
// queued. Test failures are collected rather than aborting the whole run.
func (w *workflow) runMutation(
	ctx context.Context,
	mutation m.Mutation,
	threadID int,
	mutationTimeout time.Duration,
	reports pkg.FileSpill[m.Report],
	errsMu *sync.Mutex,
	errs *[]error,
) error {
	w.progress.DisplayStartingTestInfo(ctx, mutation, threadID)

	mutationCtx := ctx

	var cancel context.CancelFunc
	if mutationTimeout > 0 {
		mutationCtx, cancel = context.WithTimeout(ctx, mutationTimeout)
	}

	result, err := w.orchestrator.TestMutation(mutationCtx, mutation)

	if cancel != nil {
		cancel()
	}

	if err != nil {
		errsMu.Lock()

		*errs = append(*errs, err)

		errsMu.Unlock()

		return nil
	}

	report := m.Report{
		Source: mutation.Source,
		Result: result,
	}
	if getMutationStatus(result, mutation) != m.Killed {
		diff := mutation.DiffCode
		report.Diff = &diff
	}

	// FileSpill.Append is safe for concurrent use, so no external lock is needed.
	if err := reports.Append(report); err != nil {
		slog.Error("failed to append report to filespill", "error", err)
		return fmt.Errorf("append report to filespill: %w", err)
	}

	w.progress.DisplayCompletedTestInfo(ctx, mutation, result)

	return nil
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
