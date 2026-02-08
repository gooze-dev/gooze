package domain

import (
	"context"
	"log/slog"
	"sort"

	"gooze.dev/pkg/gooze/internal/adapter"
	m "gooze.dev/pkg/gooze/internal/model"
)

// MutationStreamer defines the interface for streaming mutation generation.
type MutationStreamer interface {
	Get(ctx context.Context, paths []m.Path, exclude []string, threads int) <-chan m.Mutation
	ShardMutations(ctx context.Context, allMutations <-chan m.Mutation, threads int, shardIndex, totalShardCount int) <-chan m.Mutation
}

type mutationStreamer struct {
	adapter.SourceFSAdapter
	Mutagen
}

// NewMutationStreamer creates a new MutationStreamer instance with the provided dependencies.
func NewMutationStreamer(fsAdapter adapter.SourceFSAdapter, mutagen Mutagen) MutationStreamer {
	return &mutationStreamer{
		SourceFSAdapter: fsAdapter,
		Mutagen:         mutagen,
	}
}

// Get streams mutations for the given paths, excluding specified patterns.
// The channel closes when done or when ctx is cancelled.
// Check ctx.Err() after channel closes to determine if an error occurred.
func (ms *mutationStreamer) Get(ctx context.Context, paths []m.Path, exclude []string, threads int) <-chan m.Mutation {
	slog.Debug("Starting mutation streaming", "paths", len(paths), "threads", threads)
	ch := make(chan m.Mutation, ms.normalizeBufferSize(threads))

	go func() {
		defer close(ch)

		sources, err := ms.SourceFSAdapter.Get(ctx, paths, exclude...)
		if err != nil {
			slog.Error("Failed to discover sources", "error", err)
			return
		}

		// Sort sources by path for deterministic ordering across processes
		sort.Slice(sources, func(i, j int) bool {
			return m.Path(sources[i].Origin.Hash) < m.Path(sources[j].Origin.Hash)
		})

		slog.Debug("Discovered sources", "count", len(sources))

		for _, source := range sources {
			if ctx.Err() != nil {
				slog.Debug("Mutation streaming cancelled")
				return
			}

			if !ms.processSource(ctx, source, ch) {
				return
			}
		}
	}()

	return ch
}

// normalizeBufferSize ensures the buffer size is at least 1.
func (ms *mutationStreamer) normalizeBufferSize(threads int) int {
	if threads <= 0 {
		return 1
	}

	return threads
}

// processSource generates mutations for a single source and sends them to the channel.
// Returns false if processing should stop.
func (ms *mutationStreamer) processSource(ctx context.Context, source m.Source, ch chan<- m.Mutation) bool {
	mutations, err := ms.GenerateMutation(ctx, source, DefaultMutations...)
	if err != nil {
		slog.Error("Failed to generate mutations", "source", source.Origin.FullPath, "error", err)
		return false
	}

	slog.Debug("Generated mutations for source", "source", source.Origin.FullPath, "count", len(mutations))

	for _, mutation := range mutations {
		select {
		case <-ctx.Done():
			return false
		case ch <- mutation:
		}
	}

	return true
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

// filterMutationsByShard filters mutations using round-robin shard assignment.
func (ms *mutationStreamer) filterMutationsByShard(ctx context.Context, in <-chan m.Mutation, out chan<- m.Mutation, shardIndex, totalShardCount int) {
	index := 0

	for mutation := range in {
		select {
		case <-ctx.Done():
			slog.Debug("Mutation sharding cancelled")
			return
		default:
		}

		if index%totalShardCount == shardIndex {
			select {
			case <-ctx.Done():
				return
			case out <- mutation:
			}
		}

		index++
	}
}
