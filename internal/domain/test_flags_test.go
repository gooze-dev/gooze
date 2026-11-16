package domain_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	domain "gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
	m "gooze.dev/pkg/gooze/internal/model"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

func TestParseTestFlags(t *testing.T) {
	cases := map[string]struct {
		entries []string
		want    []string
		err     string
	}{
		"none":                  {},
		"one per entry":         {entries: []string{"-short", "-race"}, want: []string{"-short", "-race"}},
		"several in one entry":  {entries: []string{"-short  -timeout=30s"}, want: []string{"-short", "-timeout=30s"}},
		"double dash":           {entries: []string{"--tags=integration"}, want: []string{"--tags=integration"}},
		"blank entries":         {entries: []string{"", "  "}},
		"a value needs =":       {entries: []string{"-skip TestSlow"}, err: `run.test_flags: "TestSlow" is not a flag; write a flag's value with =, as in -skip=TestSlow`},
		"a package":             {entries: []string{"./..."}, err: `run.test_flags: "./..." is not a flag`},
		"gooze's own -json":     {entries: []string{"-json"}, err: "run.test_flags: gooze sets -json itself or can't run with it"},
		"gooze's own -count":    {entries: []string{"-count=2"}, err: "run.test_flags: gooze sets -count itself or can't run with it"},
		"gooze's own -overlay":  {entries: []string{"--overlay=o.json"}, err: "run.test_flags: gooze sets -overlay itself or can't run with it"},
		"gooze's own -vet":      {entries: []string{"-vet=all"}, err: "run.test_flags: gooze sets -vet itself or can't run with it"},
		"no test binary output": {entries: []string{"-c"}, err: "run.test_flags: gooze sets -c itself or can't run with it"},
		"no output file":        {entries: []string{"-o=x.test"}, err: "run.test_flags: gooze sets -o itself or can't run with it"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			flags, err := domain.ParseTestFlags(tc.entries)
			if tc.err != "" {
				require.EqualError(t, err, tc.err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.want, flags)
		})
	}
}

func TestTestFlagTags(t *testing.T) {
	assert.Nil(t, domain.TestFlagTags(nil))
	assert.Nil(t, domain.TestFlagTags([]string{"-short", "-run=Tags", "-ldflags=-tags=x"}), "no -tags flag")
	assert.Equal(t, []string{"a", "b"}, domain.TestFlagTags([]string{"-short", "-tags=a,b"}))
	assert.Equal(t, []string{"a"}, domain.TestFlagTags([]string{"-tags=a", "-timeout=30s", "-v"}), "a flag that sorts after tags is not it")
	assert.Equal(t, []string{"c"}, domain.TestFlagTags([]string{"--tags=a", "-tags=c", "-race"}), "the last -tags wins, as in go")
	assert.Equal(t, []string{""}, domain.TestFlagTags([]string{"-tags="}), "an empty -tags still replaces the tags of GOFLAGS")
}

// recordingBuildTags records the build tags it was given.
type recordingBuildTags struct {
	set  bool
	tags []string
}

func (r *recordingBuildTags) SetBuildTags(tags []string) {
	r.set, r.tags = true, tags
}

// failingScan returns a source adapter whose scan fails, after recording in
// seen the tags that tags held when it started.
func failingScan(t *testing.T, tags *recordingBuildTags, seen *[]string) *domainmocks.MockSourceFSAdapter {
	t.Helper()

	fs := domainmocks.NewMockSourceFSAdapter(t)
	fs.EXPECT().Stream(mock.Anything, mock.Anything).RunAndReturn(func(context.Context, []m.Path, ...string) (<-chan rpt.Source, <-chan error) {
		*seen = tags.tags

		sources, errs := make(chan rpt.Source), make(chan error, 1)
		errs <- errors.New("scan failed")

		close(sources)
		close(errs)

		return sources, errs
	})

	return fs
}

func TestWorkflow_Estimate_BuildTagsAreSetBeforeTheSourcesAreScanned(t *testing.T) {
	tags := &recordingBuildTags{}

	var seen []string

	reporter := domainmocks.NewMockReporter(t)
	reporter.EXPECT().StartEstimate(mock.Anything).Return(nil)
	reporter.EXPECT().DisplayEstimation(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	reporter.EXPECT().Wait(mock.Anything).Return().Maybe()
	reporter.EXPECT().Close(mock.Anything).Return()

	wf := domain.NewWorkflow(failingScan(t, tags, &seen), domainmocks.NewMockReportStore(t), reporter, domainmocks.NewMockOrchestrator(t), domainmocks.NewMockMutagen(t), domain.WithBuildTags(tags))
	_ = wf.Estimate(context.Background(), domain.EstimateArgs{Paths: []m.Path{"./..."}, BuildTags: []string{"integration"}})

	assert.Equal(t, []string{"integration"}, seen)
}

func TestWorkflow_Test_BuildTagsAreSetBeforeTheSourcesAreScanned(t *testing.T) {
	tags := &recordingBuildTags{}

	var seen []string

	reporter := domainmocks.NewMockReporter(t)
	reporter.EXPECT().StartTest(mock.Anything).Return(nil)
	reporter.EXPECT().DisplayConcurrencyInfo(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	reporter.EXPECT().Wait(mock.Anything).Return().Maybe()
	reporter.EXPECT().Close(mock.Anything).Return()

	wf := domain.NewWorkflow(failingScan(t, tags, &seen), domainmocks.NewMockReportStore(t), reporter, domainmocks.NewMockOrchestrator(t), domainmocks.NewMockMutagen(t), domain.WithBuildTags(tags))
	err := wf.Test(context.Background(), domain.TestArgs{
		EstimateArgs:    domain.EstimateArgs{Paths: []m.Path{"./..."}, BuildTags: []string{"integration"}},
		Threads:         1,
		TotalShardCount: 1,
	})

	require.Error(t, err)
	assert.Equal(t, []string{"integration"}, seen)
}

func TestWorkflow_BuildTagsNeedNoReceiver(t *testing.T) {
	reporter := domainmocks.NewMockReporter(t)
	reporter.EXPECT().StartEstimate(mock.Anything).Return(errors.New("no ui"))

	wf := domain.NewWorkflow(domainmocks.NewMockSourceFSAdapter(t), domainmocks.NewMockReportStore(t), reporter, domainmocks.NewMockOrchestrator(t), domainmocks.NewMockMutagen(t))
	require.EqualError(t, wf.Estimate(context.Background(), domain.EstimateArgs{BuildTags: []string{"integration"}}), "no ui")
}
