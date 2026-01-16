package domain

import (
	"fmt"
	"sync"

	"github.com/mouse-blink/gooze/internal/adapter"
	m "github.com/mouse-blink/gooze/internal/model"
	"golang.org/x/sync/errgroup"
)

// DefaultMutations defines the default mutation types to be applied.
// DefaultMutations defines the default set of mutation types to generate.
var DefaultMutations = []m.MutationType{m.MutationArithmetic, m.MutationBoolean}

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
	Orchestrator
	Mutagen
}

// NewWorkflow creates a new WorkflowV2 instance with the provided dependencies.
func NewWorkflow(
	fsAdapter adapter.SourceFSAdapter,
	reportStore adapter.ReportStore,
	orchestrator Orchestrator,
	mutagen Mutagen,
) Workflow {
	return &workflow{
		SourceFSAdapter: fsAdapter,
		ReportStore:     reportStore,
		Orchestrator:    orchestrator,
		Mutagen:         mutagen,
	}
}

func (w *workflow) Estimate(args EstimateArgs) error {
	allMutations, err := w.GetMutations(args.Paths)
	if err != nil {
		return fmt.Errorf("generate mutations: %w", err)
	}

	for _, mutation := range allMutations {
		fmt.Printf("Mutation ID: %d, Type: %v, Source: %s\n", mutation.ID, mutation.Type, mutation.Source.Origin.Path)
	}

	return nil
}

func (w *workflow) Test(args TestArgs) error {
	allMutations, err := w.GetMutations(args.Paths)
	if err != nil {
		return fmt.Errorf("generate mutations: %w", err)
	}

	shardMutations := w.ShardMutations(allMutations, args.ShardIndex, args.TotalShardCount)

	reports, err := w.TestReports(shardMutations, args.Threads)
	if err != nil {
		return fmt.Errorf("run mutation tests: %w", err)
	}

	for _, report := range reports {
		fmt.Printf("Source: %s\n", report.Source.Origin.Path)

		for mutationType, results := range report.Result {
			for _, result := range results {
				fmt.Printf("Mutation ID: %s, Type: %v, Status: %v\n", result.MutationID, mutationType, result.Status)
			}
		}
	}

	err = w.SaveReports(args.Reports, reports)
	if err != nil {
		return fmt.Errorf("save reports: %w", err)
	}

	return nil
}

func (w *workflow) GetMutations(paths []m.Path) ([]m.Mutation, error) {
	sources, err := w.Get(paths)
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
	if totalShardCount == 0 {
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

	var (
		reportsMutex sync.Mutex
		errorsMutex  sync.Mutex
	)

	var group errgroup.Group
	if threads > 0 {
		group.SetLimit(threads)
	}

	for _, mutation := range allMutations {
		currentMutation := mutation

		group.Go(func() error {
			mutationResult, err := w.TestMutation(currentMutation)
			if err != nil {
				errorsMutex.Lock()

				errors = append(errors, err)

				errorsMutex.Unlock()

				return nil
			}

			report := m.Report{
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
