package domain_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	domain "gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
	m "gooze.dev/pkg/gooze/internal/model"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

func TestWorkflow_Estimate_CacheWithoutAReportsDirChecksNothing(t *testing.T) {
	ctx := context.Background()
	sources := []rpt.Source{{Origin: &rpt.File{FullPath: "test.go", Hash: "hash1"}}}

	fs := domainmocks.NewMockSourceFSAdapter(t)
	fs.EXPECT().Stream(ctx, mock.Anything).Return(streamSources(sources))

	mutagen := domainmocks.NewMockMutagen(t)
	mutagen.EXPECT().StreamMutations(ctx, mock.Anything, mock.Anything, typeArgs(domain.DefaultMutations)...).
		RunAndReturn(streamMutationsFn([]m.Mutation{{ID: "hash-0", Source: sources[0], Type: mgen.Arithmetic}}))

	reporter := domainmocks.NewMockReporter(t)
	reporter.EXPECT().StartEstimate(ctx).Return(nil)
	reporter.EXPECT().DisplayEstimation(ctx, mock.MatchedBy(func(e domain.Estimation) bool {
		return e.Total == 1 && e.Cached == 0
	}), nil).Return(nil)
	reporter.EXPECT().Wait(ctx).Return()
	reporter.EXPECT().Close(ctx).Return()

	// The store has no expectations: with no reports dir there is no cache to check.
	wf := domain.NewWorkflow(fs, domainmocks.NewMockReportStore(t), reporter, domainmocks.NewMockOrchestrator(t), mutagen)

	require.NoError(t, wf.Estimate(ctx, domain.EstimateArgs{Paths: []m.Path{"test.go"}, UseCache: true}))
}
