package domain

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/mouse-blink/gooze/internal/adapter"
	"github.com/mouse-blink/gooze/internal/controller"
	m "github.com/mouse-blink/gooze/internal/model"
	"golang.org/x/sync/errgroup"
)

// DefaultMutations defines the default mutation types to be applied.
// DefaultMutations defines the default set of mutation types to generate.
var DefaultMutations = []m.MutationType{m.MutationArithmetic, m.MutationBoolean, m.MutationComparison, m.MutationLogical, m.MutationUnary}

// DefaultIgnorePattern excludes generated mock files by prefix.
const DefaultIgnorePattern = "^(mick_|mock_)"

// EstimateArgs contains the arguments for estimating mutations.
type EstimateArgs struct {
	Paths    []m.Path
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

// Workflow defines the interface for the mutation testing workflow.
type Workflow interface {
	Estimate(args EstimateArgs) error
	Test(args TestArgs) error
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

	allMutations, err := w.GetMutations(args.Paths)
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
	// Start with test execution mode
	if err := w.Start(controller.WithTestMode()); err != nil {
		return err
	}
	defer w.Close()

	w.DisplayConcurencyInfo(args.Threads, args.ShardIndex, args.TotalShardCount)

	allMutations, err := w.GetMutations(args.Paths)
	if err != nil {
		return fmt.Errorf("generate mutations: %w", err)
	}

	shardMutations := w.ShardMutations(allMutations, args.ShardIndex, args.TotalShardCount)
	w.DusplayUpcomingTestsInfo(len(shardMutations))

	reports, err := w.TestReports(shardMutations, args.Threads)
	if err != nil {
		return fmt.Errorf("run mutation tests: %w", err)
	}

	err = w.SaveReports(args.Reports, reports)
	if err != nil {
		return fmt.Errorf("save reports: %w", err)
	}

	// Wait for UI to be closed by user (press 'q')
	w.Wait()

	return nil
}

func (w *workflow) GetMutations(paths []m.Path) ([]m.Mutation, error) {
	sources, err := w.Get(paths, DefaultIgnorePattern)
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
		mutations, err := w.GenerateMutation(source, mutationsIndex, DefaultMutations...)
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
		if mutation.ID%totalShardCount == shardIndex {
			shardMutations = append(shardMutations, mutation)
		}
	}

	return shardMutations
}

func (w *workflow) TestReports(allMutations []m.Mutation, threads int) ([]m.Report, error) {
	reports := []m.Report{}
	errors := []error{}

	effectiveThreads := threads
	if effectiveThreads < 1 {
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
		if entry.MutationID == fmt.Sprintf("%d", mutation.ID) {
			return entry.Status
		}
	}

	return entries[0].Status
}
