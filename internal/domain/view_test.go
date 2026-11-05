package domain_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
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
		Score: domain.MutationScore{Summary: rpt.Summary{Total: 4, Killed: 1, Survived: 1, NotCovered: 1, Error: 1}, Files: []domain.FileSummary{
			{Path: "cmd/b.go", Summary: rpt.Summary{Total: 1, NotCovered: 1}},
			{Path: "pkg/a.go", Summary: rpt.Summary{Total: 3, Killed: 1, Survived: 1, Error: 1}},
		}},
		Files: []domain.FileSummary{
			{Path: "cmd/b.go", Total: 1, NotCovered: 1},
			{Path: "pkg/a.go", Total: 3, Killed: 1, Survived: 1, Error: 1},
		},
		Survivors: []domain.Survivor{
			{Path: "cmd/b.go", Mutagen: "boolean", Status: rpt.NotCovered, MutationID: "b-1"},
			{Path: "pkg/a.go", Line: 10, Mutagen: "comparison", Status: rpt.Survived, Original: "if n < lo {", Mutated: "if n <= lo {", MutationID: "a-2"},
		},
		BuildErrors: []domain.Survivor{
			{Path: "pkg/a.go", Line: 10, Mutagen: "arithmetic", Status: rpt.Error, Original: "if n < lo {", Mutated: "if n <= lo {", MutationID: "a-3"},
		},
	}, summary)
}

func TestWorkflow_View_ByLineGroupsSurvivorsOfALine(t *testing.T) {
	dir := t.TempDir()
	diff := []byte(survivorDiff)
	other := []byte("--- original\n+++ mutated\n@@ -2,1 +2,1 @@\n-a\n+b\n")
	source := rpt.Source{Origin: &rpt.File{ShortPath: "a.go", FullPath: "/p/a.go", Hash: "a"}}

	writeReports(t, dir,
		m.Report{Source: source, Result: m.Result{mgen.Comparison: {{MutationID: "same-1", Status: rpt.Survived}}}, Diff: &diff},
		m.Report{Source: source, Result: m.Result{mgen.Numbers: {{MutationID: "same-2", Status: rpt.Survived}}}, Diff: &diff},
		m.Report{Source: source, Result: m.Result{mgen.Comparison: {{MutationID: "other", Status: rpt.Survived}}}, Diff: &other},
	)

	summary := viewSummary(t, domain.ViewArgs{Reports: m.Path(dir), ByLine: true})

	require.Len(t, summary.Survivors, 3, "the flat list stays, for the score and the machine formats")
	require.Len(t, summary.SurvivorLines, 2)
	assert.Equal(t, "a.go", summary.SurvivorLines[0].Path)
	assert.Equal(t, 2, summary.SurvivorLines[0].Line)
	assert.Equal(t, 10, summary.SurvivorLines[1].Line)
	assert.Len(t, summary.SurvivorLines[1].Survivors, 2)
}

func TestWorkflow_View_SortsBuildErrorsByLocation(t *testing.T) {
	dir := t.TempDir()
	second := []byte("--- original\n+++ mutated\n@@ -2,1 +2,1 @@\n-a\n+b\n")
	first := []byte("--- original\n+++ mutated\n@@ -1,1 +1,1 @@\n-a\n+b\n")
	source := rpt.Source{Origin: &rpt.File{ShortPath: "a.go", FullPath: "/p/a.go", Hash: "a"}}

	writeReports(t, dir,
		m.Report{Source: source, Result: m.Result{mgen.Arithmetic: {{MutationID: "second", Status: rpt.Error}}}, Diff: &second},
		m.Report{Source: source, Result: m.Result{mgen.Arithmetic: {{MutationID: "first", Status: rpt.Error}}}, Diff: &first},
	)

	summary := viewSummary(t, domain.ViewArgs{Reports: m.Path(dir)})

	require.Len(t, summary.BuildErrors, 2)
	assert.Equal(t, "first", summary.BuildErrors[0].MutationID)
	assert.Equal(t, "second", summary.BuildErrors[1].MutationID)
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
	reporter.EXPECT().DisplayMutationScore(ctx, domain.MutationScore{Summary: rpt.Summary{Total: 1, NotCovered: 1}, Files: []domain.FileSummary{
		{Path: "cmd/b.go", Summary: rpt.Summary{Total: 1, NotCovered: 1}},
	}}).Return().Once()

	wf := domain.NewWorkflow(nil, adapter.NewReportStore(), reporter, nil, nil)
	require.NoError(t, wf.View(ctx, domain.ViewArgs{Reports: m.Path(dir), Paths: []m.Path{"cmd/b.go"}, All: true}))
}

func TestWorkflow_Merge_MissingReportsDirReturnsError(t *testing.T) {
	ctx := context.Background()
	missing := filepath.Join(t.TempDir(), "missing")

	wf := domain.NewWorkflow(nil, adapter.NewReportStore(), domainmocks.NewMockReporter(t), nil, nil)
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
	assert.Equal(t, []domain.Survivor{{Path: "a.go", Mutagen: "boolean", Status: rpt.Survived, MutationID: "a-1"}}, summary.Survivors)
}

func TestWorkflow_View_EmptyMainReportsPointAtUnmergedOnes(t *testing.T) {
	for name, tt := range map[string]struct {
		dirs []string
		want string
	}{
		"nothing": {},
		"shards": {
			dirs: []string{"shard_0", "shard_1"},
			want: "found shard_0, shard_1 but no merged reports; run: gooze report merge -o %s",
		},
		"run": {
			dirs: []string{"run"},
			want: "the last --diff or --run reports are not merged; see them with: gooze report view -o %s/run",
		},
		"shards first": {
			dirs: []string{"run", "shard_0"},
			want: "found shard_0 but no merged reports; run: gooze report merge -o %s",
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			for _, sub := range tt.dirs {
				require.NoError(t, os.Mkdir(filepath.Join(dir, sub), 0o750))
			}

			want := tt.want
			if want != "" {
				want = fmt.Sprintf(want, dir)
			}

			assert.Equal(t, want, viewSummary(t, domain.ViewArgs{Reports: m.Path(dir)}).Hint)
		})
	}
}

func TestWorkflow_View_ReportsLeaveNoHint(t *testing.T) {
	dir := viewFixture(t)
	require.NoError(t, os.Mkdir(filepath.Join(dir, "run"), 0o750))

	assert.Empty(t, viewSummary(t, domain.ViewArgs{Reports: m.Path(dir)}).Hint)
}

func TestWorkflow_View_SummaryTopAndSections(t *testing.T) {
	dir := m.Path(viewFixture(t))

	summary := viewSummary(t, domain.ViewArgs{Reports: dir, Summary: true, Top: 1, Sections: true})

	assert.True(t, summary.Brief)
	assert.True(t, summary.Sections)
	assert.Equal(t, []domain.FileSummary{{Path: "cmd/b.go", Total: 1, NotCovered: 1}}, summary.Files)
	assert.Equal(t, 1, summary.HiddenFiles)
	assert.Len(t, summary.Score.Files, 2, "thresholds still check every file")

	full := viewSummary(t, domain.ViewArgs{Reports: dir, Top: 2})
	assert.Len(t, full.Files, 2)
	assert.Zero(t, full.HiddenFiles, "a top as large as the files hides none")

	negative := viewSummary(t, domain.ViewArgs{Reports: dir, Top: -1})
	assert.Len(t, negative.Files, 2, "a negative top lists every file")
	assert.False(t, full.Brief)
}

// viewOutput runs a view in format and returns what it writes.
func viewOutput(t *testing.T, dir m.Path, format domain.ViewFormat) string {
	t.Helper()

	var out bytes.Buffer

	wf := domain.NewWorkflow(nil, adapter.NewReportStore(), domainmocks.NewMockReporter(t), nil, nil)
	require.NoError(t, wf.View(context.Background(), domain.ViewArgs{Reports: dir, Format: format, Output: &out}))

	return out.String()
}

func TestWorkflow_View_WritesEveryFormatFromOneLoad(t *testing.T) {
	dir := m.Path(viewFixture(t))

	var (
		metrics, gitlab, text bytes.Buffer
		shown, written        domain.ReportSummary
	)

	reporter := domainmocks.NewMockReporter(t)
	reporter.EXPECT().DisplayReportSummary(mock.Anything, mock.Anything).
		Run(func(_ context.Context, s domain.ReportSummary) { shown = s }).Once()
	reporter.EXPECT().WriteReportSummary(mock.Anything, &text, mock.Anything).
		Run(func(_ context.Context, _ io.Writer, s domain.ReportSummary) { written = s }).Return(nil).Once()

	wf := domain.NewWorkflow(nil, adapter.NewReportStore(), reporter, nil, nil)
	require.NoError(t, wf.View(context.Background(), domain.ViewArgs{
		Reports: dir, Summary: true, Top: 1, Sections: true,
		Writes: []domain.ReportWrite{
			{Format: domain.FormatMetrics, Output: &metrics},
			{Format: domain.FormatGitLab, Output: &gitlab},
			{Format: domain.FormatText, Output: &text},
		},
	}))

	assert.Equal(t, viewOutput(t, dir, domain.FormatMetrics), metrics.String())
	assert.Equal(t, viewOutput(t, dir, domain.FormatGitLab), gitlab.String())

	assert.True(t, shown.Brief, "stdout keeps the view's shape")
	assert.Len(t, shown.Files, 1)
	assert.False(t, written.Brief, "a text file holds the whole report")
	assert.False(t, written.Sections)
	assert.Zero(t, written.HiddenFiles)
	assert.Len(t, written.Files, 2)
	assert.Len(t, written.Survivors, 2)
}

func TestWorkflow_View_WritesBeforeTheThresholdFails(t *testing.T) {
	var metrics bytes.Buffer

	reporter := domainmocks.NewMockReporter(t)
	reporter.EXPECT().DisplayReportSummary(mock.Anything, mock.Anything).Once()

	wf := domain.NewWorkflow(nil, adapter.NewReportStore(), reporter, nil, nil)
	err := wf.View(context.Background(), domain.ViewArgs{
		Reports:    m.Path(viewFixture(t)),
		Thresholds: domain.Thresholds{Key: "threshold", Total: 90},
		Writes:     []domain.ReportWrite{{Format: domain.FormatMetrics, Output: &metrics}},
	})

	require.EqualError(t, err, "mutation score 33.33% is below threshold.total 90%")
	assert.Contains(t, metrics.String(), "# EOF")
}
