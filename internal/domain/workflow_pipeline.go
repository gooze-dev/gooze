package domain

import (
	"context"

	"gooze.dev/pkg/gooze/internal/adapter"
	m "gooze.dev/pkg/gooze/internal/model"
)

type EstimateOptions struct {
	Paths        []m.Path
	ExcludePaths []string
	UseCache     bool
	ReportsPath  m.Path
}

type WorkflowPipeline interface {
	Estimate(ctx context.Context, events m.Events, options EstimateOptions) error
}

type workflowPipeline struct {
	events m.Events
	fileAdapter adapter.FilesAdapter
	mutagen Mutagen
}

func NewWorkflowPipeline(
	events m.Events,
	fileAdapter adapter.FilesAdapter,
	mutagen Mutagen,
) WorkflowPipeline {
	return &workflowPipeline{
		events:      events,
		fileAdapter: fileAdapter,
		mutagen:     mutagen,
	}
}

func (p *workflowPipeline) Estimate(ctx context.Context, events m.Events, options EstimateOptions) error {
	events.StartEstimating()

	// Pipeline stages for estimation:
	// Stage 1: Get files (from fileAdapter)
	sourceChannel, err := p.fileAdapter.Get(ctx, options.Paths, 4, options.ExcludePaths...)
	if err != nil {
		events.FinishEstimating()
		return err
	}

	// Stage 2: Generate mutations (from mutagen)
	mutationsChannel, err := p.mutagen.GenerateMutationChannel(ctx, sourceChannel, 4, DefaultMutations...)
	if err != nil {
		events.FinishEstimating()
		return err
	}

	// Stage 3: Process and emit estimation events
	// This stage runs concurrently with previous stages via channels
	err = p.stageProcessEstimations(ctx, mutationsChannel, events)

	events.FinishEstimating()
	return err
}

// stageProcessEstimations processes mutations and emits estimation events
func (p *workflowPipeline) stageProcessEstimations(ctx context.Context, mutationsChannel <-chan m.Mutation, events m.Events) error {
	estimations := map[string]struct{
		Count int
		Source m.Source
	}{}
	for mutation := range mutationsChannel {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		events.Estimating(mutation)

		// read-modify-write because you cannot assign to a field of a struct stored in a map directly
		e := estimations[*&mutation.Source.Origin.Hash]
		e.Count++
		e.Source = mutation.Source
		estimations[*&mutation.Source.Origin.Hash] = e

	}
	events.ShowEstimationResult(estimations)
	return nil
}
