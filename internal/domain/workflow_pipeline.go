package domain

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"golang.org/x/sync/errgroup"
	"gooze.dev/pkg/gooze/internal/adapter"
	"gooze.dev/pkg/gooze/internal/controller"
	m "gooze.dev/pkg/gooze/internal/model"
)

type workflowPipeline struct {
	adapter.ReportStore
	adapter.SourceFSAdapter
	controller.UI
	Orchestrator
	Mutagen
}

// NewWorkflowPipeline creates a new Workflow instance using pipeline pattern with the provided dependencies.
func NewWorkflowPipeline(
	fsAdapter adapter.SourceFSAdapter,
	reportStore adapter.ReportStore,
	ui controller.UI,
	orchestrator Orchestrator,
	mutagen Mutagen,
) Workflow {
	return &workflowPipeline{
		SourceFSAdapter: fsAdapter,
		ReportStore:     reportStore,
		UI:              ui,
		Orchestrator:    orchestrator,
		Mutagen:         mutagen,
	}
}

// Deprecated: Estimate implements the estimation workflow using a pipeline pattern.
//
//	It collects sources, applies caching logic, generates mutations,
//	and displays the estimation results. This method is currently used
//	by the 'list' command and may be refactored in the future to separate concerns more cleanly.
func (w *workflowPipeline) Estimate(args EstimateArgs) error {
	ctx := context.Background()
	// Use 1 thread for estimate mode by default
	threads := 1

	if err := w.Start(controller.WithEstimateMode()); err != nil {
		slog.Error("Failed to start workflow UI", "error", err)
		return err
	}

	allMutations, err := w.collectMutations(ctx, args, threads)
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

func (w *workflowPipeline) collectMutations(ctx context.Context, args EstimateArgs, threads int) ([]m.Mutation, error) {
	// Get sources channel
	sourcesChannel, sourcesErrorChannel := w.GetChannel(ctx, args.Paths, threads, args.Exclude...)

	// Apply cache filtering if needed
	changedSourcesChannel, changedErrorChannel := w.getChangedSourcesChannel(ctx, args, threads, sourcesChannel)

	// Generate mutations from sources
	mutationsChannel, mutationsErrorChannel := w.generateMutationsChannel(ctx, changedSourcesChannel, threads)

	// Merge all error channels
	errorChannel := mergeErrorChannels(
		mergeErrorChannels(sourcesErrorChannel, changedErrorChannel),
		mutationsErrorChannel,
	)

	// Collect all mutations and handle errors
	var (
		allMutations []m.Mutation
		collectErr   error
	)

	// Use errgroup to handle both collection and error monitoring
	group, groupCtx := errgroup.WithContext(ctx)

	// Goroutine to collect mutations
	group.Go(func() error {
		for {
			select {
			case <-groupCtx.Done():
				return groupCtx.Err()
			case mutation, ok := <-mutationsChannel:
				if !ok {
					return nil
				}

				allMutations = append(allMutations, mutation)
			}
		}
	})

	// Goroutine to monitor errors
	group.Go(func() error {
		select {
		case <-groupCtx.Done():
			return groupCtx.Err()
		case err, ok := <-errorChannel:
			if !ok {
				return nil
			}

			if err != nil {
				return err
			}

			return nil
		}
	})

	collectErr = group.Wait()
	if collectErr != nil {
		return nil, collectErr
	}

	return allMutations, nil
}

func (w *workflowPipeline) getChangedSourcesChannel(ctx context.Context, args EstimateArgs, threads int, sources <-chan m.Source) (<-chan m.Source, <-chan error) {
	changedChannel := make(chan m.Source, threads)
	errorChannel := make(chan error, 1)

	go func() {
		defer close(changedChannel)
		defer close(errorChannel)

		if !args.UseCache || args.Reports == "" {
			// No caching, just pass through all sources
			for {
				select {
				case <-ctx.Done():
					errorChannel <- ctx.Err()
					return
				case source, ok := <-sources:
					if !ok {
						return
					}

					select {
					case <-ctx.Done():
						errorChannel <- ctx.Err()
						return
					case changedChannel <- source:
					}
				}
			}
		}

		// Collect all sources for cache checking
		var allSources []m.Source

		for {
			select {
			case <-ctx.Done():
				errorChannel <- ctx.Err()
				return
			case source, ok := <-sources:
				if !ok {
					goto process
				}

				allSources = append(allSources, source)
			}
		}

	process:
		changedSources, err := w.getChangedSources(args, allSources)

		if err != nil {
			select {
			case <-ctx.Done():
				errorChannel <- ctx.Err()
			case errorChannel <- err:
			}

			return
		}

		for _, source := range changedSources {
			select {
			case <-ctx.Done():
				errorChannel <- ctx.Err()
				return
			case changedChannel <- source:
			}
		}
	}()

	return changedChannel, errorChannel
}

func (w *workflowPipeline) getChangedSources(args EstimateArgs, sources []m.Source) ([]m.Source, error) {
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

func (w *workflowPipeline) buildSourcePathMap(sources []m.Source) map[string]m.Source {
	currentByPath := map[string]m.Source{}

	for _, src := range sources {
		if src.Origin != nil && src.Origin.FullPath != "" {
			currentByPath[string(src.Origin.FullPath)] = src
		}
	}

	return currentByPath
}

func (w *workflowPipeline) separateDeletedAndChanged(changed []m.Source, currentByPath map[string]m.Source) ([]m.Source, []m.Source) {
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

func (w *workflowPipeline) generateMutationsChannel(ctx context.Context, sourcesChannel <-chan m.Source, threads int) (<-chan m.Mutation, <-chan error) {
	mutationsChannel := make(chan m.Mutation, threads)
	errorChannel := make(chan error, threads)

	var group errgroup.Group
	group.SetLimit(threads)

	go func() {
		for {
			select {
			case <-ctx.Done():
				// Context cancelled, drain remaining sources
				for range sourcesChannel {
				}

				return
			case source, ok := <-sourcesChannel:
				if !ok {
					// Channel closed, wait for all workers to finish
					err := group.Wait()

					close(mutationsChannel)

					if err != nil {
						errorChannel <- err
					}

					close(errorChannel)

					return
				}

				currentSource := source

				group.Go(func() error {
					mutations, err := w.GenerateMutation(currentSource, DefaultMutations...)
					if err != nil {
						return fmt.Errorf("generate mutations for source %s: %w", currentSource.Origin.FullPath, err)
					}

					for _, mutation := range mutations {
						select {
						case <-ctx.Done():
							return ctx.Err()
						case mutationsChannel <- mutation:
						}
					}

					return nil
				})
			}
		}
	}()

	return mutationsChannel, errorChannel
}

func (w *workflowPipeline) Test(args TestArgs) error {
	return errors.New("Test method not implemented yet")
}

func (w *workflowPipeline) View(args ViewArgs) error {
	return errors.New("View method not implemented yet")
}

func (w *workflowPipeline) Merge(args MergeArgs) error {
	return errors.New("Merge method not implemented yet")
}

func mergeErrorChannels(ch1, ch2 <-chan error) <-chan error {
	merged := make(chan error, 1)

	go func() {
		defer close(merged)

		for ch1 != nil || ch2 != nil {
			select {
			case err, ok := <-ch1:
				if !ok {
					ch1 = nil
				} else {
					merged <- err
					return // Send first error and close
				}
			case err, ok := <-ch2:
				if !ok {
					ch2 = nil
				} else {
					merged <- err
					return
				}
			}
		}
	}()

	return merged
}
