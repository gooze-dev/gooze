package domain_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"gooze.dev/pkg/gooze/internal/adapter"
	"gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
	m "gooze.dev/pkg/gooze/internal/model"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// thresholdRun runs a.go of the cache fixture with args changed by edit,
// and returns the run's displayed score and error. Its stored results are 2
// of 3 killed (66.67%); with the cache off, its one mutation survives.
func thresholdRun(t *testing.T, edit func(*domain.TestArgs)) (domain.MutationScore, error) {
	t.Helper()

	fx := newCacheFixture(t)

	var (
		score      domain.MutationScore
		estimation domain.Estimation
	)

	mutations := map[string][]m.Mutation{"a.go": {{ID: "a-1", Source: fx.a, Type: mgen.Arithmetic}}}
	wf := cachedRunWorkflow(t, []rpt.Source{fx.a}, mutations, &score, &estimation)

	args := cachedTestArgs(fx.reports)
	edit(&args)

	err := wf.Test(context.Background(), args)

	return score, err
}

func TestWorkflow_Test_ChecksThresholdsAfterShowingTheScore(t *testing.T) {
	score, err := thresholdRun(t, func(args *domain.TestArgs) {
		args.Thresholds = domain.Thresholds{Key: "threshold", Total: 70}
	})
	require.EqualError(t, err, "mutation score 66.67% is below threshold.total 70%")
	assert.Equal(t, m.ExitBelowThreshold, exitCodeOf(err))
	assert.Equal(t, 3, score.Total, "the score is shown before the check")

	_, err = thresholdRun(t, func(args *domain.TestArgs) {
		args.Thresholds = domain.Thresholds{Key: "threshold", Total: 66.67}
	})
	require.NoError(t, err)
}

func TestWorkflow_Test_AShardSkipsTheThresholds(t *testing.T) {
	shard := func(thresholds domain.Thresholds, shards int) func(*domain.TestArgs) {
		return func(args *domain.TestArgs) {
			args.UseCache = false
			args.TotalShardCount = shards
			args.Thresholds = thresholds
		}
	}
	strict := domain.Thresholds{Key: "threshold", Total: 100}

	score, err := thresholdRun(t, shard(strict, 2))
	require.NoError(t, err)
	assert.True(t, score.ThresholdsSkipped)

	score, err = thresholdRun(t, shard(domain.Thresholds{}, 2))
	require.NoError(t, err)
	assert.False(t, score.ThresholdsSkipped, "nothing to skip without thresholds")

	score, err = thresholdRun(t, shard(strict, 1))
	require.EqualError(t, err, "mutation score 0.00% is below threshold.total 100%", "an unsharded run checks them")
	assert.False(t, score.ThresholdsSkipped)
}

// The view fixture scores pkg/a.go 50% (1 of 2; one mutation doesn't
// compile) and cmd/b.go 0%: 33.33% in total.
func TestWorkflow_View_ChecksThresholds(t *testing.T) {
	dir := viewFixture(t)

	reporter := domainmocks.NewMockReporter(t)
	reporter.EXPECT().DisplayReportSummary(mock.Anything, mock.Anything).Return()
	wf := domain.NewWorkflow(nil, adapter.NewReportStore(), reporter, nil, nil)

	err := wf.View(context.Background(), domain.ViewArgs{Reports: m.Path(dir), Thresholds: domain.Thresholds{Key: "threshold", File: 40}})
	require.EqualError(t, err, "cmd/b.go: score 0.00% is below threshold.file 40%")
	assert.Equal(t, m.ExitBelowThreshold, exitCodeOf(err))

	require.NoError(t, wf.View(context.Background(), domain.ViewArgs{Reports: m.Path(dir), Paths: []m.Path{"pkg/a.go"}, Thresholds: domain.Thresholds{Key: "threshold", File: 40}}),
		"only the viewed files count")
}

func TestWorkflow_ViewAll_ChecksThresholds(t *testing.T) {
	ctx := context.Background()
	dir := viewFixture(t)

	reporter := domainmocks.NewMockReporter(t)
	reporter.EXPECT().StartTest(ctx).Return(nil)
	reporter.EXPECT().Close(ctx).Return()
	reporter.EXPECT().Wait(ctx).Return().Maybe()
	reporter.EXPECT().DisplayUpcomingTestsInfo(ctx, mock.Anything).Return()
	reporter.EXPECT().DisplayStartingTestInfo(ctx, mock.Anything, mock.Anything).Return()
	reporter.EXPECT().DisplayCompletedTestInfo(ctx, mock.Anything, mock.Anything).Return()
	reporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return()

	wf := domain.NewWorkflow(nil, adapter.NewReportStore(), reporter, nil, nil)
	err := wf.View(ctx, domain.ViewArgs{Reports: m.Path(dir), All: true, Thresholds: domain.Thresholds{Key: "threshold", Average: 30}})
	require.EqualError(t, err, "average file score 25.00% is below threshold.average 30%")
	assert.Equal(t, m.ExitBelowThreshold, exitCodeOf(err))
}

func TestWorkflow_Merge_ChecksThresholds(t *testing.T) {
	ctx := context.Background()
	dir := viewFixture(t)

	reporter := domainmocks.NewMockReporter(t)
	reporter.EXPECT().StartTest(ctx).Return(nil)
	reporter.EXPECT().Close(ctx).Return()
	reporter.EXPECT().Wait(ctx).Return().Maybe()
	reporter.EXPECT().DisplayMutationScore(ctx, mock.Anything).Return()

	wf := domain.NewWorkflow(nil, adapter.NewReportStore(), reporter, nil, nil)
	err := wf.Merge(ctx, domain.MergeArgs{Reports: m.Path(dir), Thresholds: domain.Thresholds{Key: "threshold", Total: 33.34}})
	require.EqualError(t, err, "mutation score 33.33% is below threshold.total 33.34%")
	assert.Equal(t, m.ExitBelowThreshold, exitCodeOf(err))

	require.NoError(t, wf.Merge(ctx, domain.MergeArgs{Reports: m.Path(dir), Thresholds: domain.Thresholds{Key: "threshold", Total: 33.33}}))
}
