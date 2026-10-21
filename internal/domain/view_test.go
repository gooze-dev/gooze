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
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

const survivorDiff = "--- original\n+++ mutated\n@@ -7,7 +7,7 @@\n \n // Clamp limits n.\n func Clamp(n, lo, hi int) int {\n-\tif n < lo {\n+\tif n <= lo {\n \t\treturn lo\n \t}\n"

// viewFixture writes reports for pkg/a.go (one killed, one survived with a
// diff, one error) and cmd/b.go (one not covered without a diff).
func viewFixture(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	diff := []byte(survivorDiff)
	a := rpt.Source{Origin: &rpt.File{ShortPath: "pkg/a.go", FullPath: "/p/pkg/a.go", Hash: "a"}}
	b := rpt.Source{Origin: &rpt.File{ShortPath: "cmd/b.go", FullPath: "/p/cmd/b.go", Hash: "b"}}

	writeReports(t, dir,
		m.Report{Source: a, Result: m.Result{mgen.Comparison: {{MutationID: "a-1", Status: rpt.Killed}}}},
		m.Report{Source: a, Result: m.Result{mgen.Comparison: {{MutationID: "a-2", Status: rpt.Survived}}}, Diff: &diff},
		m.Report{Source: a, Result: m.Result{mgen.Arithmetic: {{MutationID: "a-3", Status: rpt.Error}}}, Diff: &diff},
		m.Report{Source: b, Result: m.Result{mgen.Boolean: {{MutationID: "b-1", Status: rpt.NotCovered}}}},
	)

	return dir
}

func viewSummary(t *testing.T, args domain.ViewArgs) domain.ReportSummary {
	t.Helper()

	var summary domain.ReportSummary

	reporter := domainmocks.NewMockReporter(t)
	reporter.EXPECT().DisplayReportSummary(mock.Anything, mock.Anything).
		Run(func(_ context.Context, s domain.ReportSummary) { summary = s }).Once()

	wf := domain.NewWorkflow(nil, adapter.NewReportStore(), reporter, nil, nil)
	require.NoError(t, wf.View(context.Background(), args))

	return summary
}

func TestWorkflow_View_SummarizesPerFileWithSurvivors(t *testing.T) {
	summary := viewSummary(t, domain.ViewArgs{Reports: m.Path(viewFixture(t))})

	assert.Equal(t, domain.ReportSummary{
		Score: domain.MutationScore{Summary: rpt.Summary{Total: 4, Killed: 1, Survived: 1, NotCovered: 1, Error: 1}},
		Files: []domain.FileSummary{
			{Path: "cmd/b.go", Total: 1, NotCovered: 1},
			{Path: "pkg/a.go", Total: 3, Killed: 1, Survived: 1, Error: 1},
		},
		Survivors: []domain.Survivor{
			{Path: "cmd/b.go", Mutagen: "boolean", Status: rpt.NotCovered},
			{Path: "pkg/a.go", Line: 10, Mutagen: "comparison", Status: rpt.Survived, Original: "if n < lo {", Mutated: "if n <= lo {"},
		},
	}, summary)
}

func TestWorkflow_View_FiltersByPath(t *testing.T) {
	dir := viewFixture(t)

	summary := viewSummary(t, domain.ViewArgs{Reports: m.Path(dir), Paths: []m.Path{"./pkg/..."}})

	require.Len(t, summary.Files, 1)
	assert.Equal(t, "pkg/a.go", summary.Files[0].Path)
	assert.Equal(t, rpt.Summary{Total: 3, Killed: 1, Survived: 1, Error: 1}, summary.Score.Summary)
	assert.Len(t, summary.Survivors, 1)
}

func TestWorkflow_View_NoMatchingPathsReturnsError(t *testing.T) {
	dir := viewFixture(t)

	wf := domain.NewWorkflow(nil, adapter.NewReportStore(), new(domainmocks.MockReporter), nil, nil)
	err := wf.View(context.Background(), domain.ViewArgs{Reports: m.Path(dir), Paths: []m.Path{"internal/...", "x.go"}})
	require.EqualError(t, err, "no reports in "+dir+" match internal/... x.go")
	assert.Equal(t, m.ExitNoReports, exitCodeOf(err))
}

func TestWorkflow_ViewAll_FiltersByPath(t *testing.T) {
	ctx := context.Background()
	dir := viewFixture(t)

	reporter := domainmocks.NewMockReporter(t)
	reporter.EXPECT().StartTest(ctx).Return(nil).Once()
	reporter.EXPECT().Wait(ctx).Return().Once()
	reporter.EXPECT().Close(ctx).Return().Once()
	reporter.EXPECT().DisplayUpcomingTestsInfo(ctx, 1).Return().Once()
	reporter.EXPECT().DisplayStartingTestInfo(ctx, mock.Anything, 0).Return().Once()
	reporter.EXPECT().DisplayCompletedTestInfo(ctx, mock.MatchedBy(func(mutation m.Mutation) bool {
		return mutation.ID == "b-1"
	}), mock.Anything).Return().Once()
	reporter.EXPECT().DisplayMutationScore(ctx, domain.MutationScore{Total: 1, NotCovered: 1}).Return().Once()

	wf := domain.NewWorkflow(nil, adapter.NewReportStore(), reporter, nil, nil)
	require.NoError(t, wf.View(ctx, domain.ViewArgs{Reports: m.Path(dir), Paths: []m.Path{"cmd/b.go"}, All: true}))
}

func TestWorkflow_Merge_MissingReportsDirReturnsError(t *testing.T) {
	ctx := context.Background()
	missing := filepath.Join(t.TempDir(), "missing")

	reporter := domainmocks.NewMockReporter(t)
	reporter.EXPECT().StartTest(ctx).Return(nil).Once()
	reporter.EXPECT().Close(ctx).Return().Once()

	wf := domain.NewWorkflow(nil, adapter.NewReportStore(), reporter, nil, nil)
	err := wf.Merge(ctx, domain.MergeArgs{Reports: m.Path(missing)})
	require.ErrorContains(t, err, "no reports directory at "+missing)
	assert.Equal(t, m.ExitNoReports, exitCodeOf(err))
}

func TestWorkflow_View_EmptyReportsDirWithoutPathsDisplaysEmptySummary(t *testing.T) {
	summary := viewSummary(t, domain.ViewArgs{Reports: m.Path(t.TempDir())})

	assert.Equal(t, domain.ReportSummary{}, summary)
}

func TestWorkflow_View_SinglePathWithoutMatchReturnsError(t *testing.T) {
	dir := viewFixture(t)

	wf := domain.NewWorkflow(nil, adapter.NewReportStore(), new(domainmocks.MockReporter), nil, nil)
	require.EqualError(t, wf.View(context.Background(), domain.ViewArgs{Reports: m.Path(dir), Paths: []m.Path{"x.go"}}),
		"no reports in "+dir+" match x.go")
}

func TestWorkflow_View_SurvivorWithoutChangedLineHasNoLocation(t *testing.T) {
	dir := t.TempDir()
	headersOnly := []byte("--- original\n+++ mutated\n")
	source := rpt.Source{Origin: &rpt.File{ShortPath: "a.go", Hash: "a"}}

	writeReports(t, dir, m.Report{Source: source, Result: m.Result{mgen.Boolean: {{MutationID: "a-1", Status: rpt.Survived}}}, Diff: &headersOnly})

	summary := viewSummary(t, domain.ViewArgs{Reports: m.Path(dir)})
	assert.Equal(t, []domain.Survivor{{Path: "a.go", Mutagen: "boolean", Status: rpt.Survived}}, summary.Survivors)
}
