package domain_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gooze.dev/pkg/gooze/internal/adapter"
	domain "gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
	m "gooze.dev/pkg/gooze/internal/model"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// mergeRun folds the run directory of reports into the main reports.
func mergeRun(t *testing.T, reports string) {
	t.Helper()

	reporter := domainmocks.NewMockReporter(t)
	for _, call := range []string{"StartTest", "Wait", "Close", "DisplayMutationScore", "DisplayReportSummary"} {
		reporter.On(call, mock.Anything).Return(nil).Maybe()
		reporter.On(call, mock.Anything, mock.Anything).Return().Maybe()
	}

	wf := domain.NewWorkflow(domainmocks.NewMockSourceFSAdapter(t), adapter.NewReportStore(), reporter,
		domainmocks.NewMockOrchestrator(t), addLessMutations)
	require.NoError(t, wf.Merge(context.Background(), domain.MergeArgs{Reports: m.Path(reports)}))
}

func TestWorkflow_Test_MergedMutagensResultsAreCached(t *testing.T) {
	reports := filepath.Join(t.TempDir(), "reports")

	arithmetic := cachedTestArgs(reports)
	arithmetic.Mutagens = []string{"arithmetic"}

	ran, _ := runAddLess(t, arithmetic)
	require.Equal(t, []string{"add-1", "add-2"}, ran)

	mergeRun(t, reports)

	ran, score := runAddLess(t, arithmetic)
	assert.Empty(t, ran, "the same --mutagens run reuses the merged results")
	assert.Equal(t, 2, score.Cached)

	ran, score = runAddLess(t, cachedTestArgs(reports))
	assert.Equal(t, []string{"less-1", "less-2"}, ran, "a full run tests only the other mutagens' mutants")
	assert.Equal(t, rpt.Summary{Total: 4, Killed: 3, Survived: 1}, score.Summary)
	assert.Equal(t, 2, score.Cached)
	assert.Equal(t, 2, score.Tested)

	for id, scope := range storedIDs(t, reports) {
		assert.Nil(t, scope, "a.go is complete: %s", id)
	}

	ran, _ = runAddLess(t, cachedTestArgs(reports))
	assert.Empty(t, ran)
}
