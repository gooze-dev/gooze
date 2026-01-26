package domain

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/mouse-blink/gooze/internal/adapter"
	"github.com/mouse-blink/gooze/internal/controller"
	m "github.com/mouse-blink/gooze/internal/model"
	"golang.org/x/sync/errgroup"
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
}

// TestArgs contains the arguments for running mutation tests.
type TestArgs struct {
	EstimateArgs
	Reports         m.Path
	Threads         int
	ShardIndex      int
	TotalShardCount int
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
		return err
	}

	allMutations, err := w.GetMutations(args)
	if err != nil {
		w.Close()
		return fmt.Errorf("generate mutations: %w", err)
	}

	err = w.DisplayEstimation(allMutations, nil)
	if err != nil {
		w.Close()
		return fmt.Errorf("display: %w", err)
	}

	// Wait for UI to be closed by user (press 'q')
	w.Wait()
	w.Close()

	return nil
}

func (w *workflow) Test(args TestArgs) error {
	return w.withTestUI(func() error {
		w.DisplayConcurencyInfo(args.Threads, args.ShardIndex, args.TotalShardCount)

		reportsDir := shardReportsDir(args.Reports, args.ShardIndex, args.TotalShardCount)

		allMutations, err := w.GetMutations(args.EstimateArgs)
		if err != nil {
			return fmt.Errorf("generate mutations: %w", err)
		}

		shardMutations := w.ShardMutations(allMutations, args.ShardIndex, args.TotalShardCount)
		w.DusplayUpcomingTestsInfo(len(shardMutations))

		reports, err := w.TestReports(shardMutations, args.Threads)
		if err != nil {
			return fmt.Errorf("run mutation tests: %w", err)
		}

		err = w.SaveReports(reportsDir, reports)
		if err != nil {
			return fmt.Errorf("save reports: %w", err)
		}

		err = w.RegenerateIndex(reportsDir)
		if err != nil {
			return fmt.Errorf("regenerate index: %w", err)
		}

		return nil
	})
}

func shardReportsDir(base m.Path, shardIndex int, totalShardCount int) m.Path {
	if totalShardCount <= 1 {
		return base
	}

	return m.Path(filepath.Join(string(base), fmt.Sprintf("%s%d", ShardDirPrefix, shardIndex)))
}

func (w *workflow) View(args ViewArgs) error {
	return w.withTestUI(func() error {
		reports, err := w.LoadReports(args.Reports)
		if err != nil {
			return fmt.Errorf("load reports: %w", err)
		}

		mutations, results := viewItemsFromReports(reports)
		if len(mutations) == 0 {
			return nil
		}

		w.DusplayUpcomingTestsInfo(len(mutations))

		for i, mutation := range mutations {
			w.DisplayStartingTestInfo(mutation, 0)
			w.DisplayCompletedTestInfo(mutation, results[i])
		}

		return nil
	})
}

func (w *workflow) Merge(args MergeArgs) error {
	base := args.Reports
	if string(base) == "" {
		return fmt.Errorf("reports directory path is required")
	}

	shardDirs, err := findShardDirs(string(base))
	if err != nil {
		return fmt.Errorf("find shard directories: %w", err)
	}

	if len(shardDirs) == 0 {
		if err := w.RegenerateIndex(base); err != nil {
			return fmt.Errorf("regenerate index: %w", err)
		}

		return nil
	}

	merged := make([]m.Report, 0)

	for _, shardDir := range shardDirs {
		reports, err := w.LoadReports(m.Path(shardDir))
		if err != nil {
			return fmt.Errorf("load shard reports from %s: %w", shardDir, err)
		}

		merged = append(merged, reports...)
	}

	if err := w.SaveReports(base, merged); err != nil {
		return fmt.Errorf("save merged reports: %w", err)
	}

	if err := w.RegenerateIndex(base); err != nil {
		return fmt.Errorf("regenerate index: %w", err)
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
	if err := w.Start(controller.WithTestMode()); err != nil {
		return err
	}
	defer w.Close()

	err := fn()
	if err != nil {
		return err
	}

	// Wait for UI to be closed by user (press 'q')
	w.Wait()

	return nil
}

func viewItemsFromReports(reports []m.Report) ([]m.Mutation, []m.Result) {
	mutations := make([]m.Mutation, 0)
	results := make([]m.Result, 0)

	for _, report := range reports {
		if len(report.Result) == 0 {
			continue
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
	}

	return mutations, results
}

func (w *workflow) GetMutations(args EstimateArgs) ([]m.Mutation, error) {
	sources, err := w.Get(args.Paths, args.Exclude...)
	if err != nil {
		return nil, fmt.Errorf("get sources: %w", err)
	}

	changedSSources, err := w.GetChangedSources(sources)
	if err != nil {
		return nil, fmt.Errorf("get changed sources: %w", err)
	}

	allMutations, err := w.GenerateAllMutations(changedSSources)
	if err != nil {
		return nil, fmt.Errorf("generate mutations: %w", err)
	}

	return allMutations, nil
}

func (w *workflow) GetChangedSources(sources []m.Source) ([]m.Source, error) {
	// Placeholder for future implementation
	return sources, nil
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

func (w *workflow) TestReports(allMutations []m.Mutation, threads int) ([]m.Report, error) {
	reports := []m.Report{}
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
		group.Go(w.processMutation(currentMutation, &threadIDCounter, effectiveThreads, &reportsMutex, &errorsMutex, &reports, &errors))
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
	reports *[]m.Report,
	errors *[]error,
) func() error {
	return func() error {
		// Assign a thread ID to this goroutine
		threadID := int(atomic.AddInt32(threadIDCounter, 1)) % threads

		w.DisplayStartingTestInfo(currentMutation, threadID)

		mutationResult, err := w.TestMutation(currentMutation)
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

		*reports = append(*reports, report)

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

	return entries[0].Status
}
