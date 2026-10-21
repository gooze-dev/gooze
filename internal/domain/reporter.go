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
	// Cached reports whether the file's stored reports are still valid, so a
	// run would reuse them instead of testing its mutations.
	Cached bool
}

// Estimation summarizes how many mutations would be generated for a run,
// broken down per source file. It carries only counts, so producing it never
// requires holding the mutations themselves in memory. Total includes the
// Cached mutations of files a run would not re-test.
type Estimation struct {
	Total  int
	Cached int
	Files  []FileEstimate
	// Diff is set in diff mode: the counts then cover changed lines only.
	Diff *DiffInfo
}

// Reporter is the presentation port the workflow depends on. It is defined in
// the domain so the domain layer does not import the controller (presentation)
// layer. Any UI implementation that satisfies this interface can be injected.
//
//nolint:interfacebloat // One presentation port keeps every workflow output in a single UI.
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
	DisplayMutationScore(ctx context.Context, score MutationScore)
	// DisplayReportSummary prints a per-file summary of stored reports. It is
	// static output: callers do not start the UI first.
	DisplayReportSummary(ctx context.Context, summary ReportSummary)
}
