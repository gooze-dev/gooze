package domain

import (
	"context"
	"log/slog"
	"sync/atomic"

	"golang.org/x/sync/errgroup"
	"gooze.dev/pkg/gooze/internal/adapter"
	m "gooze.dev/pkg/gooze/internal/model"
)

// MutationStreamer defines the interface for streaming mutation generation.
type MutationStreamer interface {
	Get(ctx context.Context, paths []m.Path, exclude []string, threads int) (<-chan m.Mutation, error)
	FilterUnchanged(ctx context.Context, mutations <-chan m.Mutation, reportsPath m.Path, threads int) <-chan m.Mutation
	ShardMutations(ctx context.Context, allMutations <-chan m.Mutation, threads int, shardIndex, totalShardCount int) <-chan m.Mutation
}

type mutationStreamer struct {
	adapter.SourceFSAdapter
	adapter.ReportStore
	Mutagen
}

// NewMutationStreamer creates a new MutationStreamer instance with the provided dependencies.
func NewMutationStreamer(fsAdapter adapter.SourceFSAdapter, reportStore adapter.ReportStore, mutagen Mutagen) MutationStreamer {
	return &mutationStreamer{
		SourceFSAdapter: fsAdapter,
		ReportStore:     reportStore,
		Mutagen:         mutagen,
	}
}

// Get streams mutations for the given paths, excluding specified patterns.
// The channel closes when done or when ctx is cancelled.
// Returns an error if source streaming fails or mutation generation encounters errors.
func (ms *mutationStreamer) Get(ctx context.Context, paths []m.Path, exclude []string, threads int) (<-chan m.Mutation, error) {
	slog.Debug("Starting mutation streaming", "paths", len(paths), "threads", threads)
	ch := make(chan m.Mutation, ms.normalizeBufferSize(threads))

	sourceCh, err := ms.SourceFSAdapter.GetStream(ctx, paths, exclude...)
	if err != nil {
		slog.Error("Failed to start source streaming", "error", err)
		close(ch)
		return ch, err
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		defer close(ch)
		defer slog.Debug("Mutation streaming completed")

		slog.Debug("Source streaming started")

		var globalIndex uint64
		for source := range sourceCh {
			if ctx.Err() != nil {
				slog.Debug("Mutation streaming cancelled")
				return ctx.Err()
			}

			if err := ms.processSource(ctx, source, ch, &globalIndex); err != nil {
				return err
			}
		}

		slog.Debug("Finished processing all sources", "totalMutations", atomic.LoadUint64(&globalIndex))
		return nil
	})

	go func() {
		if err := g.Wait(); err != nil {
			slog.Error("Mutation streaming failed", "error", err)
		}
	}()

	return ch, nil
}

func (ms *mutationStreamer) normalizeBufferSize(threads int) int {
	if threads <= 0 {
		return 1
	}
	return threads * 2 // Buffer size is typically set to a multiple of the number of threads for better throughput
}

// FilterUnchanged filters mutations by checking if their sources have changed since the last report.
// It uses the ReportStore to load cached reports and only passes through mutations for changed sources.
// If reportsPath is empty, all mutations are passed through (no filtering).
func (ms *mutationStreamer) FilterUnchanged(ctx context.Context, mutations <-chan m.Mutation, reportsPath m.Path, threads int) <-chan m.Mutation {
	ch := make(chan m.Mutation, ms.normalizeBufferSize(threads))

	go func() {
		defer close(ch)
		if ctx.Err() != nil {
			slog.Debug("Mutation filtering cancelled before start")
			return
		}
		var newIndex uint64
		for mutation := range mutations {
			if ctx.Err() != nil {
				slog.Debug("Mutation filtering cancelled")
				return
			}
			source := mutation.Source
			changed, err := ms.ReportStore.CheckUpdate(ctx, reportsPath, source)
			if err != nil {
				slog.Error("Failed to check source update", "source", source.Origin.FullPath, "error", err)
				continue
			}
			if changed {
				mutation.Index = atomic.AddUint64(&newIndex, 1) - 1

				select {
				case <-ctx.Done():
					slog.Debug("Mutation filtering cancelled during processing")
					return
				case ch <- mutation:
				}
			} else {
				slog.Debug("Source unchanged, skipping mutation", "source", source.Origin.FullPath)
			}
		}
	}()

	return ch
}

// processSource generates mutations for a single source and sends them to the channel.
// Returns an error if mutation generation fails or context is cancelled.
func (ms *mutationStreamer) processSource(ctx context.Context, source m.Source, ch chan<- m.Mutation, globalIndex *uint64) error {
	mutations, err := ms.GenerateMutation(ctx, source, DefaultMutations...)
	if err != nil {
		slog.Error("Failed to generate mutations", "source", source.Origin.FullPath, "error", err)
		return err
	}

	slog.Debug("Generated mutations for source", "source", source.Origin.FullPath, "count", len(mutations))

	for i := range mutations {
		mutations[i].Index = atomic.AddUint64(globalIndex, 1) - 1

		select {
		case <-ctx.Done():
			return ctx.Err()
		case ch <- mutations[i]:
		}
	}

	return nil
}

// ShardMutations filters mutations by shard index using hash-based distribution.
// It streams only mutations that belong to the specified shard.
func (ms *mutationStreamer) ShardMutations(ctx context.Context, allMutations <-chan m.Mutation, threads int, shardIndex, totalShardCount int) <-chan m.Mutation {
	ch := make(chan m.Mutation, ms.normalizeBufferSize(threads))

	go func() {
		defer close(ch)

		// If sharding is disabled, pass through all mutations
		if totalShardCount <= 0 {
			slog.Debug("Sharding disabled, passing through all mutations")
			ms.passThroughMutations(ctx, allMutations, ch)

			return
		}

		slog.Debug("Starting mutation sharding", "shardIndex", shardIndex, "totalShardCount", totalShardCount)
		ms.filterMutationsByShard(ctx, allMutations, ch, shardIndex, totalShardCount)
	}()

	return ch
}

// passThroughMutations forwards all mutations from input to output channel.
func (ms *mutationStreamer) passThroughMutations(ctx context.Context, in <-chan m.Mutation, out chan<- m.Mutation) {
	for mutation := range in {
		select {
		case <-ctx.Done():
			slog.Debug("Mutation pass-through cancelled")
			return
		case out <- mutation:
		}
	}
}

// filterMutationsByShard filters mutations using global index-based shard assignment.
func (ms *mutationStreamer) filterMutationsByShard(ctx context.Context, in <-chan m.Mutation, out chan<- m.Mutation, shardIndex, totalShardCount int) {
	for mutation := range in {
		select {
		case <-ctx.Done():
			slog.Debug("Mutation sharding cancelled")
			return
		default:
		}

		if int(mutation.Index)%totalShardCount == shardIndex {
			select {
			case <-ctx.Done():
				return
			case out <- mutation:
			}
		}
	}
}
