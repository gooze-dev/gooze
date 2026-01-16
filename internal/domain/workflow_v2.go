package domain

import (
	"fmt"
	"sync"

	"github.com/mouse-blink/gooze/internal/adapter"
	m "github.com/mouse-blink/gooze/internal/model"
	"golang.org/x/sync/errgroup"
)

// TestArgs contains the arguments for running mutation tests.
type TestArgs struct {
	Paths           []m.Path
	Reports         m.Path
	Threads         uint
	ShardIndex      uint
	TotalShardCount uint
}

// WorkflowV2 defines the interface for the mutation testing workflow.
type WorkflowV2 interface {
	Test(args TestArgs) error
}

type workflowV2 struct {
	adapter.ReportStore
	adapter.SourceFSAdapter
	Orchestrator
	Mutagen
}

// NewWorkflowV2 creates a new WorkflowV2 instance with the provided dependencies.
func NewWorkflowV2(
	fsAdapter adapter.SourceFSAdapter,
	reportStore adapter.ReportStore,
	orchestrator Orchestrator,
	mutagen Mutagen,
) WorkflowV2 {
	return &workflowV2{
		SourceFSAdapter: fsAdapter,
		ReportStore:     reportStore,
		Orchestrator:    orchestrator,
		Mutagen:         mutagen,
	}
}

func (w *workflowV2) Test(args TestArgs) error {
	sources, err := w.Get(args.Paths)
	if err != nil {
		return fmt.Errorf("get sources: %w", err)
	}

	allMutations, err := w.GenerateAllMutations(sources)
	if err != nil {
		return fmt.Errorf("generate mutations: %w", err)
	}

	shardMutations := w.ShardMutations(allMutations, args.ShardIndex, args.TotalShardCount)

	reports, err := w.TestReports(shardMutations, args.Threads)
	if err != nil {
		return fmt.Errorf("run mutation tests: %w", err)
	}

	err = w.SaveReports(args.Reports, reports)
	if err != nil {
		return fmt.Errorf("save reports: %w", err)
	}

	return nil
}

func (w *workflowV2) GetChangedSources(sources []m.SourceV2) ([]m.SourceV2, error) {
	// Placeholder for future implementation
	return sources, nil
}

func (w *workflowV2) GenerateAllMutations(sources []m.SourceV2) ([]m.MutationV2, error) {
	mutationsIndex := 0

	var allMutations []m.MutationV2

	for _, source := range sources {
		mutations, err := w.GenerateMutationV2(source, mutationsIndex)
		if err != nil {
			return nil, err
		}

		mutationsIndex += len(mutations)
		allMutations = append(allMutations, mutations...)
	}

	return allMutations, nil
}

func (w *workflowV2) ShardMutations(allMutations []m.MutationV2, shardIndex uint, totalShardCount uint) []m.MutationV2 {
	if totalShardCount == 0 {
		return allMutations
	}

	var shardMutations []m.MutationV2

	for _, mutation := range allMutations {
		if mutation.ID%totalShardCount == shardIndex {
			shardMutations = append(shardMutations, mutation)
		}
	}

	return shardMutations
}

func (w *workflowV2) TestReports(allMutations []m.MutationV2, threads uint) ([]m.ReportV2, error) {
	reports := []m.ReportV2{}
	errors := []error{}

	var (
		reportsMutex sync.Mutex
		errorsMutex  sync.Mutex
	)

	var group errgroup.Group
	if threads > 0 {
		group.SetLimit(int(threads))
	}

	for _, mutation := range allMutations {
		currentMutation := mutation

		group.Go(func() error {
			mutationResult, err := w.TestMutationV2(currentMutation)
			if err != nil {
				errorsMutex.Lock()

				errors = append(errors, err)

				errorsMutex.Unlock()

				return nil
			}

			report := m.ReportV2{
				Source: currentMutation.Source,
				Result: mutationResult,
			}

			reportsMutex.Lock()

			reports = append(reports, report)

			reportsMutex.Unlock()

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return reports, err
	}

	if len(errors) > 0 {
		return reports, fmt.Errorf("errors occurred during mutation testing: %v", errors)
	}

	return reports, nil
}
