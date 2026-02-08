package domain

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"gooze.dev/pkg/gooze/internal/adapter"
	"gooze.dev/pkg/gooze/internal/controller"
	m "gooze.dev/pkg/gooze/internal/model"
)

// DefaultMutations defines the default set of mutation types to generate.
var DefaultMutations = []m.MutationType{m.MutationArithmetic, m.MutationBoolean, m.MutationNumbers, m.MutationComparison, m.MutationLogical, m.MutationUnary}

// ShardDirPrefix is the directory name prefix used when storing sharded reports.
const ShardDirPrefix = "shard_"

// EstimateArgs contains the arguments for estimating mutations.
type EstimateArgs struct {
	Paths           []m.Path
	Exclude         []string
	UseCache        bool
	Reports         m.Path
	ShardIndex      int
	TotalShardCount int
}

// TestArgs contains the arguments for running mutation tests.
type TestArgs struct {
	EstimateArgs
	Threads         int
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
	MutationStreamer
	Orchestrator
	ReportManager
}

// NewWorkflow creates a new WorkflowV2 instance with the provided dependencies.
func NewWorkflow(
	fsAdapter adapter.SourceFSAdapter,
	reportStore adapter.ReportStore,
	ui controller.UI,
	orchestrator Orchestrator,
	mutagen Mutagen,
	mutationStreamer MutationStreamer,
) Workflow {
	return &workflow{
		MutationStreamer: mutationStreamer,
		Orchestrator:     orchestrator,
		ReportManager:    NewReportManager(reportStore),
	}
}

// Estimate implements Workflow.
func (w *workflow) Estimate(ctx context.Context, args EstimateArgs) error {
	threads := 1 // Use a single thread for estimation to ensure consistent mutation indexing
	allMutations, err := w.MutationStreamer.Get(ctx, args.Paths, args.Exclude, threads)
	if err != nil {
		return err
	}
	var filtered <-chan m.Mutation
	if args.UseCache {
		slog.Debug("Using cache for mutation estimation")
		filtered = w.MutationStreamer.FilterUnchanged(ctx, allMutations, args.Reports, threads)
	} else {
		slog.Debug("Not using cache for mutation estimation")
		filtered = allMutations
	}
	shardMutations := w.MutationStreamer.ShardMutations(ctx, filtered, threads, args.ShardIndex, args.TotalShardCount)
	for mutation := range shardMutations {
		fmt.Printf("Estimated mutation: %s (Index: %d, Source: %s)\n", mutation.Type, mutation.Index, *&mutation.Source.Origin.FullPath)
	}
	return nil
}

// Test implements Workflow.
func (w *workflow) Test(ctx context.Context, args TestArgs) error {
	threads := 1 // Use a single thread for estimation to ensure consistent mutation indexing
	allMutations, err := w.MutationStreamer.Get(ctx, args.Paths, args.Exclude, threads)
	if err != nil {
		return err
	}
	var filtered <-chan m.Mutation
	if args.UseCache {
		slog.Debug("Using cache for mutation estimation")
		filtered = w.MutationStreamer.FilterUnchanged(ctx, allMutations, args.Reports, threads)
	} else {
		slog.Debug("Not using cache for mutation estimation")
		filtered = allMutations
	}
	shardMutations := w.MutationStreamer.ShardMutations(ctx, filtered, threads, args.ShardIndex, args.TotalShardCount)
	reportCh := w.Orchestrator.TestMutationStream(ctx, shardMutations, args.MutationTimeout)
	if err := w.ReportManager.SaveStream(ctx, args.Reports, reportCh); err != nil {
		return fmt.Errorf("failed to save reports: %w", err)
	}
	return nil
}

// Merge implements Workflow.
func (w *workflow) Merge(ctx context.Context, args MergeArgs) error {
	return fmt.Errorf("unimplemented")
}

// View implements Workflow.
func (w *workflow) View(ctx context.Context, args ViewArgs) error {
	return fmt.Errorf("unimplemented")
}
