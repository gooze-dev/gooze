package domain

import (
	"context"

	m "gooze.dev/pkg/gooze/internal/model"
)

// FileEstimate is the number of mutations that would be generated for a single
// source file.
type FileEstimate struct {
	Path  string
	Count int
}

// Estimation summarizes how many mutations would be generated for a run,
// broken down per source file. It carries only counts, so producing it never
// requires holding the mutations themselves in memory.
type Estimation struct {
	Total int
	Files []FileEstimate
}

// Reporter is the presentation port the workflow depends on. It is defined in
// the domain so the domain layer does not import the controller (presentation)
// layer. Any UI implementation that satisfies this interface can be injected.
type Reporter interface {
	StartEstimate(ctx context.Context) error
	StartTest(ctx context.Context) error
	Close(ctx context.Context)
	Wait(ctx context.Context)
	DisplayEstimation(ctx context.Context, estimation Estimation, err error) error
	DisplayConcurrencyInfo(ctx context.Context, threads, shardIndex, shardCount int)
	DisplayUpcomingTestsInfo(ctx context.Context, total int)
	DisplayStartingTestInfo(ctx context.Context, mutation m.Mutation, threadID int)
	DisplayCompletedTestInfo(ctx context.Context, mutation m.Mutation, result m.Result)
	DisplayMutationScore(ctx context.Context, score float64)
}
