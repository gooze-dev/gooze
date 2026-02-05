package domain

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
	"gooze.dev/pkg/gooze/internal/adapter"
	"gooze.dev/pkg/gooze/internal/controller"
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
	Estimate(args EstimateArgs) error
	Test(args TestArgs) error
	View(args ViewArgs) error
	Merge(args MergeArgs) error
}

type workflow struct {
	adapter.ReportStore
	adapter.SourceFSAdapter
	controller.UI
	Orchestrator
	Mutagen
}

// NewWorkflow creates a new WorkflowV2 instance with the provided dependencies.
func NewWorkflow(
	fsAdapter adapter.SourceFSAdapter,
	reportStore adapter.ReportStore,
	ui controller.UI,
	orchestrator Orchestrator,
	mutagen Mutagen,
) Workflow {
	return &workflow{
		SourceFSAdapter: fsAdapter,
		ReportStore:     reportStore,
		UI:              ui,
		Orchestrator:    orchestrator,
		Mutagen:         mutagen,
	}
}

func (w *workflow) Estimate(args EstimateArgs) error {
	if err := w.Start(controller.WithEstimateMode()); err != nil {
		slog.Error("Failed to start workflow UI", "error", err)
		return err
	}

	allMutations, err := w.GetMutations(args)
	if err != nil {
		w.Close()
		slog.Error("Failed to generate mutations", "error", err)

		return fmt.Errorf("generate mutations: %w", err)
	}

	err = w.DisplayEstimation(allMutations, nil)
	if err != nil {
		w.Close()
		slog.Error("Failed to display estimation", "error", err)

		return fmt.Errorf("display: %w", err)
	}

	// Wait for UI to be closed by user (press 'q')
	w.Wait()
	w.Close()

	return nil
}

func (w *workflow) Test(args TestArgs) error {
	return w.withTestUI(func() error {
		slog.Info("Starting mutation testing", "threads", args.Threads, "shardIndex", args.ShardIndex, "totalShardCount", args.TotalShardCount)
		w.DisplayConcurrencyInfo(args.Threads, args.ShardIndex, args.TotalShardCount)

		reportsDir := shardReportsDir(args.Reports, args.ShardIndex, args.TotalShardCount)
		slog.Debug("Using reports directory", "path", reportsDir)

		allMutations, err := w.GetMutations(args.EstimateArgs)
		if err != nil {
			slog.Error("Failed to generate mutations", "error", err)
			return fmt.Errorf("generate mutations: %w", err)
		}

		slog.Debug("Generated mutations", "count", len(allMutations))
		shardMutations := w.ShardMutations(allMutations, args.ShardIndex, args.TotalShardCount)
		slog.Debug("Sharded mutations", "count", len(shardMutations))
		w.DisplayUpcomingTestsInfo(len(shardMutations))

		reports, err := w.TestReports(shardMutations, args.Threads, args.MutationTimeout)
		if err != nil {
			slog.Error("Failed to run mutation tests", "error", err)
			return fmt.Errorf("run mutation tests: %w", err)
		}

		slog.Info("Completed mutation tests", "reportsCount", reports.Len())

		score, err := mutationScoreFromReports(reports)
		if err != nil {
			return fmt.Errorf("calculate mutation score: %w", err)
		}

		slog.Info("Calculated mutation score", "score", score)
		w.DisplayMutationScore(score)

		err = w.SaveSpillReports(reportsDir, reports)
		if err != nil {
			slog.Error("Failed to save reports", "error", err, "path", reportsDir)
			return fmt.Errorf("save reports: %w", err)
		}

		err = reports.Close()
		if err != nil {
			slog.Error("Failed to close reports spill", "error", err, "path", reportsDir)
			return fmt.Errorf("close reports spill: %w", err)
		}

		slog.Debug("Saved reports", "path", reportsDir)

		err = w.RegenerateIndex(reportsDir)
		if err != nil {
			slog.Error("Failed to regenerate reports index", "error", err, "path", reportsDir)
			return fmt.Errorf("regenerate index: %w", err)
		}

		slog.Debug("Regenerated reports index", "path", reportsDir)

		return nil
	})
}

func shardReportsDir(base m.Path, shardIndex int, totalShardCount int) m.Path {
	if totalShardCount <= 1 {
		slog.Debug("Using unsharded reports directory", "path", base)
		return base
	}

	slog.Debug("Using sharded reports directory", "base", base, "shardIndex", shardIndex)

	return m.Path(filepath.Join(string(base), fmt.Sprintf("%s%d", ShardDirPrefix, shardIndex)))
}

func (w *workflow) View(args ViewArgs) error {
	return w.withTestUI(func() error {
		slog.Info("Loading mutation test reports", "path", args.Reports)

		reports, err := w.LoadSpillReports(args.Reports)
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
		w.DisplayUpcomingTestsInfo(len(mutations))

		for i, mutation := range mutations {
			w.DisplayStartingTestInfo(mutation, 0)
			w.DisplayCompletedTestInfo(mutation, results[i])
		}

		w.DisplayMutationScore(score)

		return nil
	})
}

func (w *workflow) Merge(args MergeArgs) error {
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
		return w.regenerateIndex(base)
	}

	merged, err := w.mergeReports(base, shardDirs)
	if err != nil {
		slog.Error("Failed to merge shard reports", "error", err, "basePath", base)
		return err
	}

	slog.Info("Merged shard reports", "basePath", base)

	if err := w.saveMergedReports(base, merged); err != nil {
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

func (w *workflow) regenerateIndex(base m.Path) error {
	if err := w.RegenerateIndex(base); err != nil {
		return fmt.Errorf("regenerate index: %w", err)
	}

	return nil
}

func (w *workflow) mergeReports(base m.Path, shardDirs []string) ([]m.Report, error) {
	merged := make([]m.Report, 0)

	// First, load existing reports from base directory to preserve cache.
	existingReports, err := w.loadReportsIfExists(base)
	if err != nil {
		return nil, fmt.Errorf("load existing reports from base: %w", err)
	}

	merged = append(merged, existingReports...)

	// Then load and merge reports from all shards.
	for _, shardDir := range shardDirs {
		reports, err := w.LoadReports(m.Path(shardDir))
		if err != nil {
			slog.Error("Failed to load shard reports", "error", err, "shardDir", shardDir)
			return nil, fmt.Errorf("load shard reports from %s: %w", shardDir, err)
		}

		merged = append(merged, reports...)
	}

	slog.Debug("Merged reports from shards", "totalReports", len(merged))

	return merged, nil
}

func (w *workflow) loadReportsIfExists(path m.Path) ([]m.Report, error) {
	reports, err := w.LoadReports(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}

		return nil, err
	}

	return reports, nil
}

func (w *workflow) saveMergedReports(base m.Path, reports []m.Report) error {
	if err := w.SaveReports(base, reports); err != nil {
		slog.Error("Failed to save merged reports", "error", err, "basePath", base)
		return fmt.Errorf("save merged reports: %w", err)
	}

	slog.Debug("Saved merged reports and regenerated index", "basePath", base)

	return w.regenerateIndex(base)
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

func (w *workflow) withTestUI(fn func() error) error {
	slog.Info("Starting workflow in test mode")

	if err := w.Start(controller.WithTestMode()); err != nil {
		slog.Error("Failed to start workflow in test mode", "error", err)
		return err
	}

	defer func() {
		slog.Info("Closing workflow UI")
		w.Close()
	}()

	err := fn()
	if err != nil {
		return err
	}

	// Wait for UI to be closed by user (press 'q')
	w.Wait()

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

func (w *workflow) GetMutations(args EstimateArgs) ([]m.Mutation, error) {
	sources, err := w.Get(args.Paths, args.Exclude...)
	if err != nil {
		return nil, fmt.Errorf("get sources: %w", err)
	}

	changedSSources, err := w.GetChangedSources(args, sources)
	if err != nil {
		return nil, fmt.Errorf("get changed sources: %w", err)
	}

	allMutations, err := w.GenerateAllMutations(changedSSources)
	if err != nil {
		return nil, fmt.Errorf("generate mutations: %w", err)
	}

	return allMutations, nil
}

func (w *workflow) GetChangedSources(args EstimateArgs, sources []m.Source) ([]m.Source, error) {
	if !args.UseCache {
		return sources, nil
	}

	if args.Reports == "" {
		return sources, nil
	}

	changed, err := w.CheckUpdates(args.Reports, sources)
	if err != nil {
		return nil, err
	}

	currentByPath := w.buildSourcePathMap(sources)
	deleted, changedExisting := w.separateDeletedAndChanged(changed, currentByPath)

	if len(deleted) > 0 {
		if err := w.CleanReports(args.Reports, deleted); err != nil {
			return nil, err
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

func (w *workflow) GenerateAllMutations(sources []m.Source) ([]m.Mutation, error) {
	mutationsIndex := 0

	var allMutations []m.Mutation

	for _, source := range sources {
		mutations, err := w.GenerateMutation(source, DefaultMutations...)
		if err != nil {
			return nil, err
		}

		mutationsIndex += len(mutations)
		allMutations = append(allMutations, mutations...)
	}

	return allMutations, nil
}

func (w *workflow) ShardMutations(allMutations []m.Mutation, shardIndex int, totalShardCount int) []m.Mutation {
	if totalShardCount <= 0 {
		return allMutations
	}

	var shardMutations []m.Mutation

	for _, mutation := range allMutations {
		// Use hash of the mutation ID to determine shard
		h := sha256.Sum256([]byte(mutation.ID))

		hashValue := int(h[0])<<24 + int(h[1])<<16 + int(h[2])<<8 + int(h[3])
		if hashValue < 0 {
			hashValue = -hashValue
		}

		if hashValue%totalShardCount == shardIndex {
			shardMutations = append(shardMutations, mutation)
		}
	}

	return shardMutations
}

<<<<<<< HEAD
func (w *workflow) TestReports(allMutations []m.Mutation, threads int, mutationTimeout time.Duration) ([]m.Report, error) {
	reports := []m.Report{}
=======
func (w *workflow) TestReports(allMutations []m.Mutation, threads int) (pkg.FileSpill[m.Report], error) {
	reports, err := pkg.NewFileSpill[m.Report]()
	if err != nil {
		slog.Error("failed to create reports filespill", "error", err)
		return nil, fmt.Errorf("create reports filespill: %w", err)
	}

>>>>>>> 34f02a5 (use filespill in workflow)
	errors := []error{}

	effectiveThreads := threads
	if effectiveThreads <= 0 {
		effectiveThreads = 1
	}

	var (
		reportsMutex    sync.Mutex
		errorsMutex     sync.Mutex
		threadIDCounter int32 = -1
	)

	var group errgroup.Group
	group.SetLimit(effectiveThreads)

	for _, mutation := range allMutations {
		currentMutation := mutation
		group.Go(w.processMutation(
			currentMutation, &threadIDCounter,
			effectiveThreads, &reportsMutex,
			&errorsMutex, &reports,
			&errors, mutationTimeout,
		))
	}

	if err := group.Wait(); err != nil {
		return reports, err
	}

	if len(errors) == 0 {
		return reports, nil
	}

	return reports, fmt.Errorf("errors occurred during mutation testing: %v", errors)
}

func (w *workflow) processMutation(
	currentMutation m.Mutation,
	threadIDCounter *int32,
	threads int,
	reportsMutex *sync.Mutex,
	errorsMutex *sync.Mutex,
	reports *pkg.FileSpill[m.Report],
	errors *[]error,
	mutationTimeout time.Duration,
) func() error {
	return func() error {
		// Assign a thread ID to this goroutine
		threadID := int(atomic.AddInt32(threadIDCounter, 1)) % threads

		w.DisplayStartingTestInfo(currentMutation, threadID)

		mutationResult, err := w.TestMutationWithTimeout(currentMutation, mutationTimeout)
		if err != nil {
			errorsMutex.Lock()

			*errors = append(*errors, err)

			errorsMutex.Unlock()

			return nil
		}

		report := m.Report{
			Source: currentMutation.Source,
			Result: mutationResult,
		}
		if getMutationStatus(mutationResult, currentMutation) == m.Survived {
			diff := currentMutation.DiffCode
			report.Diff = &diff
		}

		reportsMutex.Lock()

		err = (*reports).Append(report)
		if err != nil {
			slog.Error("failed to append report to filespill", "error", err)
			return fmt.Errorf("append report to filespill: %w", err)
		}

		reportsMutex.Unlock()

		w.DisplayCompletedTestInfo(currentMutation, mutationResult)

		return nil
	}
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
