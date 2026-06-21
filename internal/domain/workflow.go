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

		sources, err := w.changedSources(ctx, args.EstimateArgs)
		if err != nil {
			slog.Error("Failed to resolve sources", "error", err)
			return err
		}

		inThisShard := func(mutation m.Mutation) bool {
			return inShard(mutation.ID, args.ShardIndex, args.TotalShardCount)
		}

		// Count mutations up front so the UI can show an accurate progress total.
		estimation, err := w.countMutations(ctx, sources, inThisShard)
		if err != nil {
			slog.Error("Failed to count mutations", "error", err)
			return fmt.Errorf("generate mutations: %w", err)
		}

		w.progress.DisplayUpcomingTestsInfo(ctx, estimation.Total)

		var gate *CoverageIndex
		if args.CoverageProfile != "" {
			gate, err = w.loadCoverage(ctx, args.CoverageProfile)
			if err != nil {
				slog.Error("Failed to load coverage profile", "error", err)
				return fmt.Errorf("load coverage profile: %w", err)
			}
		}

		reports, err := w.testReports(ctx, sources, inThisShard, gate, args.Threads, args.MutationTimeout)
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

	if err := w.removeShardDirs(shardDirs); err != nil {
		return err
	}

	// Report the combined score last, so `gooze report merge` ends with it.
	w.progress.DisplayMutationScore(ctx, mutationScoreFromReportSlice(merged))

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
		for _, mutationType := range sortedResultTypes(report.Result) {
			for _, entry := range report.Result[mutationType] {
				mutation := m.Mutation{ID: entry.MutationID, Source: report.Source, Type: mutationType}
				if entry.Status == m.Survived && report.Diff != nil {
					mutation.DiffCode = *report.Diff
				}

				mutations = append(mutations, mutation)
				results = append(results, m.Result{mutationType: {entry}})
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

// changedSources resolves the sources to operate on, applying incremental
// caching (skipping unchanged files and cleaning reports for deleted ones) when
// enabled. The scanner streams its results; they are collected into a slice
// (sources are lightweight) so the count and test passes can both iterate them.
func (w *workflow) changedSources(ctx context.Context, args EstimateArgs) ([]m.Source, error) {
	sources, err := w.scanSources(ctx, args)
	if err != nil {
		return nil, err
	}

	if !args.UseCache || args.Reports == "" {
		return sources, nil
	}

	changed, err := w.reports.CheckUpdates(ctx, args.Reports, sources)
	if err != nil {
		return nil, fmt.Errorf("check updates: %w", err)
	}

	deleted, changedExisting := w.separateDeletedAndChanged(changed, w.buildSourcePathMap(sources))

	if len(deleted) > 0 {
		if err := w.reports.CleanReports(ctx, args.Reports, deleted); err != nil {
			return nil, fmt.Errorf("clean reports: %w", err)
		}
	}

	return changedExisting, nil
}

// scanSources drains the streaming scanner into a slice.
func (w *workflow) scanSources(ctx context.Context, args EstimateArgs) ([]m.Source, error) {
	sourceCh, scanErrc := w.sources.Stream(ctx, args.Paths, args.Exclude...)

	//nolint:prealloc // the stream length is unknown ahead of time.
	var sources []m.Source
	for source := range sourceCh {
		sources = append(sources, source)
	}

	if err := <-scanErrc; err != nil {
		return nil, fmt.Errorf("get sources: %w", err)
	}

	return sources, nil
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

// estimateMutations streams sources and mutations and returns per-file counts,
// without ever holding the sources or mutations in memory all at once.
func (w *workflow) estimateMutations(ctx context.Context, args EstimateArgs) (Estimation, error) {
	sources, err := w.changedSources(ctx, args)
	if err != nil {
		return Estimation{}, err
	}

	return w.countMutations(ctx, sources, nil)
}

// countMutations generates the mutations for the given sources and aggregates
// per-file counts. include, when non-nil, filters which mutations are counted.
// Mutations are processed one at a time and never retained.
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
	content, err := w.sources.ReadFile(ctx, profile)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", profile, err)
	}

	return ParseCoverage(content)
}

func (w *workflow) testReports(
	ctx context.Context,
	sources []m.Source,
	include func(m.Mutation) bool,
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

	group.Go(w.dispatchMutations(ctx, sources, include, gate, queues, results))

	for threadID := range effectiveThreads {
		group.Go(w.consumeMutations(ctx, queues[threadID], threadID, mutationTimeout, results))
	}

	runErr := group.Wait()

	close(results)

	return reports, testRunError(runErr, <-collected)
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

// dispatchMutations returns a task that generates each source's mutations,
// shards inline, and dispatches each kept mutation to a ready worker queue,
// closing every queue when done so the workers terminate.
func (w *workflow) dispatchMutations(
	ctx context.Context,
	sources []m.Source,
	include func(m.Mutation) bool,
	gate *CoverageIndex,
	queues []chan m.Mutation,
	results chan<- mutationOutcome,
) func() error {
	return func() error {
		defer closeQueues(queues)

		for _, source := range sources {
			err := w.mutagen.StreamMutations(ctx, source, func(mutation m.Mutation) error {
				if include != nil && !include(mutation) {
					return nil
				}

				if gate != nil && !gate.Covers(string(mutation.Source.Origin.ShortPath), mutation.Line) {
					return sendOutcome(ctx, results, mutationOutcome{
						mutation: mutation,
						result:   resultForStatus(mutation, m.NotCovered),
					})
				}

				return dispatch(ctx, queues, mutation)
			}, DefaultMutations...)
			if err != nil {
				return fmt.Errorf("generate mutations: %w", err)
			}
		}

		return nil
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
