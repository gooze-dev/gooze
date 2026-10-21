package domain_test

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"gooze.dev/pkg/gooze/internal/adapter"
	domain "gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
	m "gooze.dev/pkg/gooze/internal/model"
	pkg "gooze.dev/pkg/gooze/pkg"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

func mergeSource(hash string) rpt.Source {
	return rpt.Source{Origin: &rpt.File{FullPath: "/p/calc.go", ShortPath: "calc.go", Hash: hash}}
}

func mergeReport(source rpt.Source, id string, status rpt.TestStatus, scope *rpt.Scope) m.Report {
	return m.Report{Source: source, Result: m.Result{mgen.Comparison: {{MutationID: id, Status: status}}}, Line: 1, Scope: scope}
}

func saveReports(t *testing.T, dir string, reports ...m.Report) {
	t.Helper()

	spill, err := pkg.NewFileSpill[m.Report]()
	require.NoError(t, err)

	for _, report := range reports {
		require.NoError(t, spill.Append(report))
	}

	require.NoError(t, adapter.NewReportStore().SaveReports(context.Background(), m.Path(dir), spill))
}

// storedResults returns "id=status" for every result in dir, sorted, and
// the ids whose report is marked partial.
func storedResults(t *testing.T, dir string) ([]string, []string) {
	t.Helper()

	reports, err := rpt.ReadDir(dir)
	require.NoError(t, err)

	var out, partial []string

	for _, report := range reports {
		for _, result := range report.Result {
			for _, mutation := range result.Mutations {
				out = append(out, mutation.MutationID+"="+mutation.Status.String())

				if report.Scope != nil {
					partial = append(partial, mutation.MutationID)
				}
			}
		}
	}

	sort.Strings(out)
	sort.Strings(partial)

	return out, partial
}

func mergeWorkflow(t *testing.T) domain.Workflow {
	t.Helper()

	reporter := domainmocks.NewMockReporter(t)
	for _, call := range []string{"StartTest", "Wait", "Close", "DisplayMutationScore"} {
		reporter.On(call, mock.Anything).Return(nil).Maybe()
		reporter.On(call, mock.Anything, mock.Anything).Return().Maybe()
	}

	return domain.NewWorkflow(domainmocks.NewMockSourceFSAdapter(t), adapter.NewReportStore(), reporter,
		domainmocks.NewMockOrchestrator(t), domainmocks.NewMockMutagen(t))
}

var runScope = &rpt.Scope{Run: "^InRange$"}

func TestMerge_RunOfUnchangedFileReplacesOnlyItsMutations(t *testing.T) {
	base := t.TempDir()
	runDir := filepath.Join(base, domain.RunDirName)

	saveReports(t, base,
		mergeReport(mergeSource("h1"), "in-a", rpt.Killed, nil),
		mergeReport(mergeSource("h1"), "in-c", rpt.Survived, nil),
		mergeReport(mergeSource("h1"), "next", rpt.Survived, nil))
	// The run re-tested InRange's survivor after a new test killed it.
	saveReports(t, runDir, mergeReport(mergeSource("h1"), "in-c", rpt.Killed, runScope))

	require.NoError(t, mergeWorkflow(t).Merge(context.Background(), domain.MergeArgs{Reports: m.Path(base)}))

	results, partial := storedResults(t, base)
	assert.Equal(t, []string{"in-a=killed", "in-c=killed", "next=survived"}, results)
	assert.Empty(t, partial, "the file is still fully tested: the run replaced matching mutations only")
	assert.NoDirExists(t, runDir)
}

func TestMerge_RunOfChangedFileReplacesTheFileAndMarksItPartial(t *testing.T) {
	base := t.TempDir()

	saveReports(t, base,
		mergeReport(mergeSource("h1"), "in-a", rpt.Killed, nil),
		mergeReport(mergeSource("h1"), "next", rpt.Survived, nil))
	// The file changed since the main reports: their results are stale.
	saveReports(t, filepath.Join(base, domain.RunDirName), mergeReport(mergeSource("h2"), "in-x", rpt.Survived, runScope))

	require.NoError(t, mergeWorkflow(t).Merge(context.Background(), domain.MergeArgs{Reports: m.Path(base)}))

	results, partial := storedResults(t, base)
	assert.Equal(t, []string{"in-x=survived"}, results)
	assert.Equal(t, []string{"in-x"}, partial, "the next full run must re-test the rest of the file")
}

func TestMerge_RunOfNewFileIsAddedAsPartial(t *testing.T) {
	base := t.TempDir()
	saveReports(t, filepath.Join(base, domain.RunDirName), mergeReport(mergeSource("h1"), "in-a", rpt.Killed, runScope))

	require.NoError(t, mergeWorkflow(t).Merge(context.Background(), domain.MergeArgs{Reports: m.Path(base)}))

	results, partial := storedResults(t, base)
	assert.Equal(t, []string{"in-a=killed"}, results)
	assert.Equal(t, []string{"in-a"}, partial)
}

func TestMerge_RunOntoPartialFileStaysPartial(t *testing.T) {
	base := t.TempDir()

	saveReports(t, base, mergeReport(mergeSource("h1"), "in-a", rpt.Killed, &rpt.Scope{Base: "abc", Lines: []rpt.LineRange{{Start: 1, End: 1}}}))
	saveReports(t, filepath.Join(base, domain.RunDirName), mergeReport(mergeSource("h1"), "next", rpt.Killed, runScope))

	require.NoError(t, mergeWorkflow(t).Merge(context.Background(), domain.MergeArgs{Reports: m.Path(base)}))

	results, partial := storedResults(t, base)
	assert.Equal(t, []string{"in-a=killed", "next=killed"}, results)
	assert.Equal(t, []string{"in-a", "next"}, partial, "untested lines stay untested, so the run's results stay partial too")
}

func TestMerge_RunShardsAreMergedToo(t *testing.T) {
	base := t.TempDir()
	runDir := filepath.Join(base, domain.RunDirName)

	saveReports(t, base, mergeReport(mergeSource("h1"), "in-a", rpt.Survived, nil), mergeReport(mergeSource("h1"), "next", rpt.Survived, nil))
	saveReports(t, filepath.Join(runDir, domain.ShardDirPrefix+"0"), mergeReport(mergeSource("h1"), "in-a", rpt.Killed, runScope))
	saveReports(t, filepath.Join(runDir, domain.ShardDirPrefix+"1"), mergeReport(mergeSource("h1"), "next", rpt.Killed, runScope))

	require.NoError(t, mergeWorkflow(t).Merge(context.Background(), domain.MergeArgs{Reports: m.Path(base)}))

	results, partial := storedResults(t, base)
	assert.Equal(t, []string{"in-a=killed", "next=killed"}, results)
	assert.Empty(t, partial)
	assert.NoDirExists(t, runDir)
}

func TestMerge_EmptyRunDirIsRemoved(t *testing.T) {
	base := t.TempDir()
	runDir := filepath.Join(base, domain.RunDirName)

	saveReports(t, base, mergeReport(mergeSource("h1"), "in-a", rpt.Killed, nil))
	require.NoError(t, os.MkdirAll(runDir, 0o750))

	require.NoError(t, mergeWorkflow(t).Merge(context.Background(), domain.MergeArgs{Reports: m.Path(base)}))

	results, _ := storedResults(t, base)
	assert.Equal(t, []string{"in-a=killed"}, results)
	assert.NoDirExists(t, runDir)
}

func TestMerge_AnyHashChangeMeansTheFileChanged(t *testing.T) {
	withTest := func(origin, test string) rpt.Source {
		source := mergeSource(origin)
		source.Test = &rpt.File{FullPath: "/p/calc_test.go", ShortPath: "calc_test.go", Hash: test}

		return source
	}

	cases := map[string]struct{ base, run rpt.Source }{
		"origin hash lower in the run":  {withTest("h2", "t1"), withTest("h1", "t1")},
		"origin hash higher in the run": {withTest("h1", "t1"), withTest("h2", "t1")},
		"test hash lower in the run":    {withTest("h1", "t2"), withTest("h1", "t1")},
		"test hash higher in the run":   {withTest("h1", "t1"), withTest("h1", "t2")},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			base := t.TempDir()
			saveReports(t, base, mergeReport(tc.base, "old", rpt.Survived, nil))
			saveReports(t, filepath.Join(base, domain.RunDirName), mergeReport(tc.run, "new", rpt.Killed, runScope))

			require.NoError(t, mergeWorkflow(t).Merge(context.Background(), domain.MergeArgs{Reports: m.Path(base)}))

			results, partial := storedResults(t, base)
			assert.Equal(t, []string{"new=killed"}, results, "stale results of a changed file are dropped")
			assert.Equal(t, []string{"new"}, partial)
		})
	}
}

func TestMerge_KeepsOnlyMainResultsTheRunDidNotReplace(t *testing.T) {
	// Checked at the port, independent of how the store names report files.
	base := t.TempDir()
	saveReports(t, filepath.Join(base, domain.RunDirName), mergeReport(mergeSource("h1"), "in-c", rpt.Killed, runScope))

	stored := []m.Report{
		mergeReport(mergeSource("h1"), "in-a", rpt.Killed, nil),
		mergeReport(mergeSource("h1"), "in-c", rpt.Survived, nil),
	}

	real := adapter.NewReportStore()
	store := domainmocks.NewMockReportStore(t)
	store.EXPECT().LoadSpillReports(mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, path m.Path) (pkg.FileSpill[m.Report], error) {
		if string(path) != base {
			return real.LoadSpillReports(ctx, path)
		}

		spill, err := pkg.NewFileSpill[m.Report]()
		require.NoError(t, err)

		for _, report := range stored {
			require.NoError(t, spill.Append(report))
		}

		return spill, nil
	})
	store.EXPECT().CleanReports(mock.Anything, mock.Anything, mock.Anything).Return(nil)
	store.EXPECT().RegenerateIndex(mock.Anything, mock.Anything).Return(nil)

	var saves [][]string

	store.EXPECT().SaveReports(mock.Anything, m.Path(base), mock.Anything).RunAndReturn(func(_ context.Context, _ m.Path, reports pkg.FileSpill[m.Report]) error {
		var ids []string

		err := reports.Range(func(_ uint64, report m.Report) error {
			for _, results := range report.Result {
				for _, result := range results {
					ids = append(ids, result.MutationID+"="+result.Status.String())
				}
			}

			return nil
		})
		saves = append(saves, ids)

		return err
	})

	reporter := domainmocks.NewMockReporter(t)
	for _, call := range []string{"StartTest", "Wait", "Close", "DisplayMutationScore"} {
		reporter.On(call, mock.Anything).Return(nil).Maybe()
		reporter.On(call, mock.Anything, mock.Anything).Return().Maybe()
	}

	wf := domain.NewWorkflow(domainmocks.NewMockSourceFSAdapter(t), store, reporter, domainmocks.NewMockOrchestrator(t), domainmocks.NewMockMutagen(t))
	require.NoError(t, wf.Merge(context.Background(), domain.MergeArgs{Reports: m.Path(base)}))

	assert.Contains(t, saves, []string{"in-a=killed"}, "the main result the run did not re-test is kept")
	assert.Contains(t, saves, []string{"in-c=killed"}, "the run's result replaces the old one")

	for _, ids := range saves {
		assert.NotContains(t, ids, "in-c=survived", "the replaced main result is not written back")
	}
}
