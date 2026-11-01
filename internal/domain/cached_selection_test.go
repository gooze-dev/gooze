package domain_test

import (
	"context"
	"path/filepath"
	"slices"
	"sync"
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

// fakeMutagen streams fixed mutations per source key, keeping those of the
// asked types.
type fakeMutagen map[string][]m.Mutation

func (f fakeMutagen) GenerateMutation(context.Context, rpt.Source, ...mgen.Type) ([]m.Mutation, error) {
	return nil, nil
}

func (f fakeMutagen) StreamMutations(_ context.Context, source rpt.Source, fn func(m.Mutation) error, types ...mgen.Type) error {
	for _, mutation := range f[source.Key()] {
		if !slices.Contains(types, mutation.Type) {
			continue
		}

		if err := fn(mutation); err != nil {
			return err
		}
	}

	return nil
}

// survivingWorkspace kills every mutant but those in survivors.
type survivingWorkspace struct {
	mu        sync.Mutex
	survivors []string
	ran       []string
}

func (ws *survivingWorkspace) Run(_ context.Context, mutation m.Mutation) (m.Result, error) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	ws.ran = append(ws.ran, mutation.ID)

	status := rpt.Killed
	if slices.Contains(ws.survivors, mutation.ID) {
		status = rpt.Survived
	}

	return m.Result{mutation.Type: {{MutationID: mutation.ID, Status: status}}}, nil
}

func (ws *survivingWorkspace) Close(context.Context) {}

// addLessSource is a.go: Add on line 3 with two arithmetic mutants, add-2
// surviving, and Less on line 4 with two comparison mutants.
var addLessSource = rpt.Source{
	Origin: &rpt.File{ShortPath: "a.go", FullPath: "/p/a.go", Hash: "a1"},
	Test:   &rpt.File{ShortPath: "a_test.go", FullPath: "/p/a_test.go", Hash: "at"},
}

// subSource is b.go, with two surviving arithmetic mutants in Sub.
var subSource = rpt.Source{Origin: &rpt.File{ShortPath: "b.go", FullPath: "/p/b.go", Hash: "b1"}}

var addLessMutations = fakeMutagen{
	"a.go": {
		{ID: "add-1", Source: addLessSource, Type: mgen.Arithmetic, Line: 3, Funcs: []string{"Add"}},
		{ID: "add-2", Source: addLessSource, Type: mgen.Arithmetic, Line: 3, Funcs: []string{"Add"}},
		{ID: "less-1", Source: addLessSource, Type: mgen.Comparison, Line: 4, Funcs: []string{"Less"}},
		{ID: "less-2", Source: addLessSource, Type: mgen.Comparison, Line: 4, Funcs: []string{"Less"}},
	},
	"b.go": {
		{ID: "sub-1", Source: subSource, Type: mgen.Arithmetic, Line: 3, Funcs: []string{"Sub"}},
		{ID: "sub-2", Source: subSource, Type: mgen.Arithmetic, Line: 3, Funcs: []string{"Sub"}},
	},
}

// runAddLess tests a.go with args and returns the mutants it ran and the
// score it displayed.
func runAddLess(t *testing.T, args domain.TestArgs) ([]string, domain.MutationScore) {
	t.Helper()

	ran, score, err := runSources(t, args, addLessSource)
	require.NoError(t, err)

	return ran, score
}

// runSources tests sources with args and returns the mutants it ran, the
// score it displayed and its error.
func runSources(t *testing.T, args domain.TestArgs, sources ...rpt.Source) ([]string, domain.MutationScore, error) {
	t.Helper()

	ctx := context.Background()

	fs := domainmocks.NewMockSourceFSAdapter(t)
	fs.EXPECT().Stream(mock.Anything, mock.Anything).Return(streamSources(sources))

	ws := &survivingWorkspace{survivors: []string{"add-2", "sub-1", "sub-2"}}
	orchestrator := domainmocks.NewMockOrchestrator(t)
	orchestrator.EXPECT().NewWorkspace(mock.Anything).Return(ws).Maybe()

	changes := domainmocks.NewMockChangeDetector(t)
	changes.EXPECT().Changes(mock.Anything, mock.Anything, "main").Return(domain.ChangeSet{
		Ref: "main", Base: "b1",
		Files: map[string]domain.FileChange{"/p/a.go": {Lines: []domain.LineRange{{Start: 3, End: 3}}}},
	}, nil).Maybe()

	var score domain.MutationScore

	reporter := domainmocks.NewMockReporter(t)
	reporter.EXPECT().StartTest(mock.Anything).Return(nil)
	reporter.EXPECT().Wait(mock.Anything).Return().Maybe()
	reporter.EXPECT().Close(mock.Anything).Return()
	reporter.EXPECT().DisplayConcurrencyInfo(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	reporter.EXPECT().DisplayUpcomingTestsInfo(mock.Anything, mock.Anything).Return()
	reporter.EXPECT().DisplayStartingTestInfo(mock.Anything, mock.Anything, mock.Anything).Return().Maybe()
	reporter.EXPECT().DisplayCompletedTestInfo(mock.Anything, mock.Anything, mock.Anything).Return().Maybe()
	reporter.EXPECT().DisplayMutationScore(mock.Anything, mock.Anything).
		Run(func(_ context.Context, s domain.MutationScore) { score = s })

	wf := domain.NewWorkflow(fs, adapter.NewReportStore(), reporter, orchestrator, addLessMutations, domain.WithChangeDetector(changes))
	err := wf.Test(ctx, args)

	return ws.ran, score, err
}

// storedIDs returns the mutation IDs of the reports in dir, with their scope.
func storedIDs(t *testing.T, dir string) map[string]*rpt.Scope {
	t.Helper()

	reports, err := adapter.NewReportStore().LoadSpillReports(context.Background(), m.Path(dir))
	require.NoError(t, err)

	defer reports.Close()

	ids := map[string]*rpt.Scope{}

	require.NoError(t, reports.Range(func(_ uint64, report m.Report) error {
		for _, results := range report.Result {
			for _, result := range results {
				ids[result.MutationID] = report.Scope
			}
		}

		return nil
	}))

	return ids
}

func TestWorkflow_Test_NarrowedRunScoresTheCachedResultsOfItsSelection(t *testing.T) {
	cases := map[string]struct {
		args  func(*domain.TestArgs)
		want  rpt.Summary
		scope *rpt.Scope
	}{
		"mutagens": {
			args:  func(a *domain.TestArgs) { a.Mutagens = []string{"arithmetic"} },
			want:  rpt.Summary{Total: 2, Killed: 1, Survived: 1},
			scope: &rpt.Scope{Mutagens: []string{"arithmetic"}},
		},
		"run": {
			args:  func(a *domain.TestArgs) { a.Run = "^Less$" },
			want:  rpt.Summary{Total: 2, Killed: 2},
			scope: &rpt.Scope{Run: "^Less$"},
		},
		"diff": {
			args:  func(a *domain.TestArgs) { a.Diff = "main" },
			want:  rpt.Summary{Total: 2, Killed: 1, Survived: 1},
			scope: &rpt.Scope{Base: "b1", Lines: []rpt.LineRange{{Start: 3, End: 3}}},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			reports := filepath.Join(t.TempDir(), "reports")

			ran, _ := runAddLess(t, cachedTestArgs(reports))
			require.Len(t, ran, 4)

			args := cachedTestArgs(reports)
			tc.args(&args)

			ran, score := runAddLess(t, args)

			assert.Empty(t, ran, "a.go is unchanged, so nothing is tested")
			assert.Equal(t, tc.want, score.Summary)
			assert.Equal(t, tc.want.Total, score.Cached)
			assert.Zero(t, score.Tested)
			assert.Equal(t, []domain.FileSummary{{Path: "a.go", Summary: tc.want}}, score.Files)

			runDir := filepath.Join(reports, domain.RunDirName)
			assert.Equal(t, m.Path(runDir), score.ReportsDir)

			stored := storedIDs(t, runDir)
			assert.Len(t, stored, tc.want.Total, "the run's reports hold its cached results")

			for id, scope := range stored {
				assert.Equal(t, tc.scope, scope, id)
			}
		})
	}
}

func TestWorkflow_Test_NarrowedRunScoresCachedAndTestedFiles(t *testing.T) {
	reports := filepath.Join(t.TempDir(), "reports")
	runAddLess(t, cachedTestArgs(reports))

	args := cachedTestArgs(reports)
	args.Mutagens = []string{"arithmetic"}

	ran, score, err := runSources(t, args, addLessSource, subSource)

	require.NoError(t, err)
	assert.Equal(t, []string{"sub-1", "sub-2"}, ran, "b.go is new, a.go is cached")
	assert.Equal(t, rpt.Summary{Total: 4, Killed: 1, Survived: 3}, score.Summary)
	assert.Equal(t, 2, score.Cached)
	assert.Equal(t, 2, score.Tested)
}

func TestWorkflow_Test_NarrowedRunFailsFastOnItsCachedSurvivors(t *testing.T) {
	reports := filepath.Join(t.TempDir(), "reports")
	runAddLess(t, cachedTestArgs(reports))

	args := cachedTestArgs(reports)
	args.Mutagens = []string{"arithmetic"}
	args.FailFast = domain.FailFastGate
	args.Thresholds = domain.Thresholds{MaxSurvivors: 1}

	ran, _, err := runSources(t, args, addLessSource, subSource)

	require.Error(t, err)
	assert.Equal(t, []string{"sub-1"}, ran, "add-2, cached, and sub-1 are already one survivor too many")
}

func TestWorkflow_Test_NarrowedRunScoresOnlyTheFilesItSelects(t *testing.T) {
	reports := filepath.Join(t.TempDir(), "reports")
	_, _, err := runSources(t, cachedTestArgs(reports), addLessSource, subSource)
	require.NoError(t, err)

	args := cachedTestArgs(reports)
	args.Run = "^Less$"

	ran, score, err := runSources(t, args, addLessSource, subSource)

	require.NoError(t, err)
	assert.Empty(t, ran)
	assert.Equal(t, []domain.FileSummary{{Path: "a.go", Summary: rpt.Summary{Total: 2, Killed: 2}}}, score.Files,
		"b.go has no mutant in Less")
}

func TestWorkflow_Test_NarrowedShardScoresOnlyItsOwnResults(t *testing.T) {
	reports := filepath.Join(t.TempDir(), "reports")
	runAddLess(t, cachedTestArgs(reports))

	args := cachedTestArgs(reports)
	args.Mutagens = []string{"arithmetic"}
	args.ShardIndex, args.TotalShardCount = 0, 2

	ran, score := runAddLess(t, args)

	assert.Empty(t, ran)
	assert.Zero(t, score.Total, "report merge scores the cached results")
}
