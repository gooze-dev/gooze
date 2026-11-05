package domain_test

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
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

func TestWorkflow_Test_NarrowedShardsSplitTheCachedResultsBySource(t *testing.T) {
	reports := filepath.Join(t.TempDir(), "reports")
	runAddLess(t, cachedTestArgs(reports))

	var scores []int

	for shard := range 2 {
		args := cachedTestArgs(reports)
		args.Mutagens = []string{"arithmetic"}
		args.ShardIndex, args.TotalShardCount = shard, 2

		ran, score := runAddLess(t, args)

		assert.Empty(t, ran)
		assert.Equal(t, score.Total, score.Cached)
		scores = append(scores, score.Cached)
	}

	assert.ElementsMatch(t, []int{0, 2}, scores, "one shard owns a.go and keeps both of its cached results")
}

// shardFixture is six sources, p0.go to p5.go, each with a mutant on line 3
// and one on line 5; the line-3 mutants of the even files survive.
type shardFixture struct {
	sources   []rpt.Source
	mutations fakeMutagen
	survivors []string
}

func newShardFixture() shardFixture {
	f := shardFixture{mutations: fakeMutagen{}}

	for i := range 6 {
		path := fmt.Sprintf("p%d.go", i)
		source := rpt.Source{Origin: &rpt.File{ShortPath: path, FullPath: "/p/" + path, Hash: "h" + path}}
		f.sources = append(f.sources, source)
		f.mutations[path] = []m.Mutation{
			{ID: path + "-3", Source: source, Type: mgen.Arithmetic, Line: 3, Funcs: []string{"F"}},
			{ID: path + "-5", Source: source, Type: mgen.Arithmetic, Line: 5, Funcs: []string{"G"}},
		}

		if i%2 == 0 {
			f.survivors = append(f.survivors, path+"-3")
		}
	}

	return f
}

// edited returns the fixture with the source at i changed since a baseline.
func (f shardFixture) edited(i int) shardFixture {
	sources := slices.Clone(f.sources)
	origin := *sources[i].Origin
	origin.Hash += "-edited"
	sources[i].Origin = &origin

	mutations := fakeMutagen{}
	for key, list := range f.mutations {
		mutations[key] = slices.Clone(list)
	}

	for j := range mutations[origin.ShortPath] {
		mutations[origin.ShortPath][j].Source = sources[i]
	}

	return shardFixture{sources: sources, mutations: mutations, survivors: f.survivors}
}

// diffOfEveryFile changes line 3 of every fixture file since main.
func (f shardFixture) diffOfEveryFile() domain.ChangeSet {
	files := map[string]domain.FileChange{}
	for _, source := range f.sources {
		files[source.Origin.FullPath] = domain.FileChange{Lines: []domain.LineRange{{Start: 3, End: 3}}}
	}

	return domain.ChangeSet{Ref: "main", Base: "b1", Files: files}
}

// scoringReporter returns a reporter that records the score a workflow displays.
func scoringReporter(t *testing.T, score *domain.MutationScore) *domainmocks.MockReporter {
	t.Helper()

	reporter := domainmocks.NewMockReporter(t)
	reporter.EXPECT().StartTest(mock.Anything).Return(nil)
	reporter.EXPECT().Wait(mock.Anything).Return().Maybe()
	reporter.EXPECT().Close(mock.Anything).Return()
	reporter.EXPECT().DisplayConcurrencyInfo(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return().Maybe()
	reporter.EXPECT().DisplayUpcomingTestsInfo(mock.Anything, mock.Anything).Return().Maybe()
	reporter.EXPECT().DisplayStartingTestInfo(mock.Anything, mock.Anything, mock.Anything).Return().Maybe()
	reporter.EXPECT().DisplayCompletedTestInfo(mock.Anything, mock.Anything, mock.Anything).Return().Maybe()
	reporter.EXPECT().DisplayMutationScore(mock.Anything, mock.Anything).
		Run(func(_ context.Context, s domain.MutationScore) { *score = s })

	return reporter
}

// run tests the fixture with args and returns the mutants it ran and its score.
func (f shardFixture) run(t *testing.T, args domain.TestArgs) ([]string, domain.MutationScore) {
	t.Helper()

	fs := domainmocks.NewMockSourceFSAdapter(t)
	fs.EXPECT().Stream(mock.Anything, mock.Anything).Return(streamSources(f.sources))

	ws := &survivingWorkspace{survivors: f.survivors}
	orchestrator := domainmocks.NewMockOrchestrator(t)
	orchestrator.EXPECT().NewWorkspace(mock.Anything).Return(ws).Maybe()

	changes := domainmocks.NewMockChangeDetector(t)
	changes.EXPECT().Changes(mock.Anything, mock.Anything, "main").Return(f.diffOfEveryFile(), nil).Maybe()

	var score domain.MutationScore

	wf := domain.NewWorkflow(fs, adapter.NewReportStore(), scoringReporter(t, &score), orchestrator, f.mutations, domain.WithChangeDetector(changes))
	require.NoError(t, wf.Test(context.Background(), args))

	return ws.ran, score
}

// merge runs gooze report merge -o dir and returns the merged score.
func (f shardFixture) merge(t *testing.T, dir string) domain.MutationScore {
	t.Helper()

	var score domain.MutationScore

	// Merge prints the merged reports, as report view does.
	reporter := domainmocks.NewMockReporter(t)
	reporter.EXPECT().DisplayReportSummary(mock.Anything, mock.Anything).
		Run(func(_ context.Context, s domain.ReportSummary) { score = s.Score }).Once()

	wf := domain.NewWorkflow(domainmocks.NewMockSourceFSAdapter(t), adapter.NewReportStore(), reporter,
		domainmocks.NewMockOrchestrator(t), f.mutations)
	require.NoError(t, wf.Merge(context.Background(), domain.MergeArgs{Reports: m.Path(dir)}))

	return score
}

// copyDir copies src to dst; a missing src copies nothing.
func copyDir(t *testing.T, src, dst string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(dst, 0o750))

	if _, err := os.Stat(src); os.IsNotExist(err) {
		return
	}

	require.NoError(t, os.CopyFS(dst, os.DirFS(src)))
}

// sourcesIn returns the source keys of the reports in dir.
func sourcesIn(t *testing.T, dir string) []string {
	t.Helper()

	reports, err := adapter.NewReportStore().LoadSpillReports(context.Background(), m.Path(dir))
	if os.IsNotExist(err) {
		return nil
	}

	require.NoError(t, err)

	defer reports.Close()

	var keys []string

	require.NoError(t, reports.Range(func(_ uint64, report m.Report) error {
		keys = append(keys, report.Source.Key())
		return nil
	}))

	return keys
}

func TestWorkflow_Test_ShardsKeepTheCachedResultsOfTheirSources(t *testing.T) {
	const shards = 3

	diff := func(a *domain.TestArgs) { a.Diff = "main" }

	cases := map[string]struct {
		baseline bool                   // a full run stores results before
		edit     bool                   // p1.go changes after the baseline
		narrow   func(*domain.TestArgs) // nil: a full run
		tested   []string               // the mutants the run tests
	}{
		"fully cached diff": {baseline: true, narrow: diff},
		"partly cached diff": {
			baseline: true, edit: true, narrow: diff,
			tested: []string{"p1.go-3"},
		},
		"diff without a baseline": {
			narrow: diff,
			tested: []string{"p0.go-3", "p1.go-3", "p2.go-3", "p3.go-3", "p4.go-3", "p5.go-3"},
		},
		"fully cached run": {
			baseline: true, narrow: func(a *domain.TestArgs) { a.Run = "^F$" },
		},
		"full run with a baseline": {baseline: true},
		"full run with a partly cached baseline": {
			baseline: true, edit: true,
			tested: []string{"p1.go-3", "p1.go-5"},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newShardFixture()
			root := t.TempDir()
			baseline := filepath.Join(root, "baseline")

			if tc.baseline {
				fixture.run(t, cachedTestArgs(baseline))
			}

			if tc.edit {
				fixture = fixture.edited(1)
			}

			argsFor := func(reports string, shard int) domain.TestArgs {
				args := cachedTestArgs(reports)
				args.ShardIndex, args.TotalShardCount = shard, shards

				if tc.narrow != nil {
					tc.narrow(&args)
				}

				return args
			}

			// resultsDir is where a run of reports keeps the results merge scores.
			resultsDir := func(reports string) string {
				if tc.narrow == nil {
					return reports
				}

				return filepath.Join(reports, domain.RunDirName)
			}

			unsharded := filepath.Join(root, "unsharded")
			copyDir(t, baseline, unsharded)

			args := argsFor(unsharded, 0)
			args.TotalShardCount = 1

			ran, want := fixture.run(t, args)
			assert.ElementsMatch(t, tc.tested, ran)

			sharded := filepath.Join(root, "sharded")
			copyDir(t, baseline, sharded)

			var (
				shardRan       []string
				cached, tested int
				owner          = map[string]int{} // cached source -> shard dir it is in
			)

			for shard := range shards {
				ran, score := fixture.run(t, argsFor(sharded, shard))
				shardRan = append(shardRan, ran...)
				cached += score.Cached
				tested += score.Tested

				assert.Equal(t, score.Total, score.Cached+score.Tested, "shard %d scores its cached results", shard)

				shardDir := filepath.Join(resultsDir(sharded), fmt.Sprintf("%s%d", domain.ShardDirPrefix, shard))
				for _, key := range sourcesIn(t, shardDir) {
					if slices.ContainsFunc(tc.tested, func(id string) bool { return strings.HasPrefix(id, key+"-") }) {
						continue // a tested source is split by mutant, as before
					}

					previous, seen := owner[key]
					assert.False(t, seen && previous != shard, "%s is in shard %d and shard %d", key, previous, shard)
					owner[key] = shard
				}
			}

			assert.ElementsMatch(t, tc.tested, shardRan, "each mutant is tested in exactly one shard")
			assert.Equal(t, want.Cached, cached, "the shards together keep every cached result")
			assert.Equal(t, want.Tested, tested)

			// CI: each shard hands only its own dir to the merge job, which
			// starts from the baseline (full run) or from nothing (narrowed run).
			ci := filepath.Join(root, "ci")
			if tc.narrow == nil {
				copyDir(t, baseline, ci)
			}

			for shard := range shards {
				dir := filepath.Join(fmt.Sprintf("%s%d", domain.ShardDirPrefix, shard))
				copyDir(t, filepath.Join(resultsDir(sharded), dir), filepath.Join(resultsDir(ci), dir))
			}

			for _, reports := range []string{sharded, ci} {
				merged := fixture.merge(t, resultsDir(reports))

				assert.Equal(t, want.Summary, merged.Summary, "%s merges to the unsharded score", reports)
				assert.Equal(t, keysOf(storedIDs(t, resultsDir(unsharded))), keysOf(storedIDs(t, resultsDir(reports))),
					"%s merges to the unsharded results", reports)
			}
		})
	}
}

func keysOf[V any](in map[string]V) []string {
	keys := slices.Collect(maps.Keys(in))
	slices.Sort(keys)

	return keys
}

func TestWorkflow_Test_AFullyCachedDiffShardShowsItsCachedResults(t *testing.T) {
	fixture := newShardFixture()
	reports := filepath.Join(t.TempDir(), "reports")
	fixture.run(t, cachedTestArgs(reports))

	var cached int

	for shard := range 3 {
		args := cachedTestArgs(reports)
		args.Diff = "main"
		args.ShardIndex, args.TotalShardCount = shard, 3

		ran, score := fixture.run(t, args)

		assert.Empty(t, ran)
		assert.Zero(t, score.Tested)
		assert.Equal(t, score.Total, score.Cached, "shard %d: N cached, 0 tested, not no mutations", shard)

		cached += score.Cached
	}

	assert.Equal(t, 6, cached, "the six line-3 mutants")
}
