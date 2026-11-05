package report

import (
	"bytes"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
	m "gooze.dev/pkg/gooze/internal/model"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

const testOutputKey = "output"

func execReport(t *testing.T, deps Deps, args ...string) error {
	t.Helper()

	cmd := New(deps)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(args)

	return cmd.Execute()
}

func TestReportView(t *testing.T) {
	viper.Set(testOutputKey, "/tmp/reports")
	defer viper.Reset()

	wf := domainmocks.NewMockWorkflow(t)
	wf.On("View", mock.Anything, domain.ViewArgs{Gate: domain.GateAuto,
		Reports: m.Path("/tmp/reports"), Paths: []m.Path{},
		Thresholds: domain.Thresholds{Key: "threshold"}, DiffThresholds: domain.Thresholds{Key: "threshold.diff"},
	}).Return(nil)

	deps := Deps{Workflow: wf, Publisher: domainmocks.NewMockReportPublisher(t), OutputKey: testOutputKey}

	require.NoError(t, execReport(t, deps, "view"))
	wf.AssertExpectations(t)
}

func TestReportView_PathsAndAll(t *testing.T) {
	viper.Set(testOutputKey, "/tmp/reports")
	defer viper.Reset()

	wf := domainmocks.NewMockWorkflow(t)
	wf.On("View", mock.Anything, domain.ViewArgs{Gate: domain.GateAuto,
		Reports:        m.Path("/tmp/reports"),
		Paths:          []m.Path{"./internal/...", "cmd/run.go"},
		All:            true,
		Thresholds:     domain.Thresholds{Key: "threshold"},
		DiffThresholds: domain.Thresholds{Key: "threshold.diff"},
	}).Return(nil)

	deps := Deps{Workflow: wf, Publisher: domainmocks.NewMockReportPublisher(t), OutputKey: testOutputKey}

	require.NoError(t, execReport(t, deps, "view", "--all", "./internal/...", "cmd/run.go"))
	wf.AssertExpectations(t)
}

func TestReportMerge(t *testing.T) {
	viper.Set(testOutputKey, "/tmp/reports")
	defer viper.Reset()

	wf := domainmocks.NewMockWorkflow(t)
	wf.On("Merge", mock.Anything, domain.MergeArgs{View: domain.ViewArgs{Gate: domain.GateAuto}, Reports: m.Path("/tmp/reports"), Thresholds: domain.Thresholds{Key: "threshold"}, DiffThresholds: domain.Thresholds{Key: "threshold.diff"}}).Return(nil)

	deps := Deps{Workflow: wf, Publisher: domainmocks.NewMockReportPublisher(t), OutputKey: testOutputKey}

	require.NoError(t, execReport(t, deps, "merge"))
	wf.AssertExpectations(t)
}

func TestReportPush(t *testing.T) {
	viper.Set(testOutputKey, "/tmp/reports")
	defer viper.Reset()

	pub := domainmocks.NewMockReportPublisher(t)
	pub.On("Push", mock.Anything, mock.MatchedBy(func(args domain.PushArgs) bool {
		return args.Ref == "ghcr.io/org/repo:tag" &&
			args.Reports == m.Path("/tmp/reports") &&
			args.PlainHTTP && !args.Insecure
	})).Return(nil)

	deps := Deps{Workflow: domainmocks.NewMockWorkflow(t), Publisher: pub, OutputKey: testOutputKey}

	require.NoError(t, execReport(t, deps, "push", "--plain-http", "ghcr.io/org/repo:tag"))
	pub.AssertExpectations(t)
}

func TestReportPull(t *testing.T) {
	viper.Set(testOutputKey, "/tmp/reports")
	defer viper.Reset()

	pub := domainmocks.NewMockReportPublisher(t)
	pub.On("Pull", mock.Anything, mock.MatchedBy(func(args domain.PullArgs) bool {
		return args.Ref == "ghcr.io/org/repo:tag" &&
			args.Reports == m.Path("/tmp/reports") &&
			args.Insecure && !args.PlainHTTP
	})).Return(domain.PullResult{
		Digest:  "sha256:abc",
		Reports: 3,
		Score:   domain.MutationScore{Summary: rpt.Summary{Total: 3, Killed: 2, Survived: 1}},
	}, nil)

	deps := Deps{Workflow: domainmocks.NewMockWorkflow(t), Publisher: pub, OutputKey: testOutputKey}

	cmd := New(deps)
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetArgs([]string{"pull", "--insecure", "ghcr.io/org/repo:tag"})

	require.NoError(t, cmd.Execute())
	assert.Equal(t, "Pulled ghcr.io/org/repo:tag (sha256:abc): 3 report files, mutation score 66.67% (2 killed, 1 survived)\n", out.String())
}

func TestReportPushRequiresReference(t *testing.T) {
	deps := Deps{Workflow: domainmocks.NewMockWorkflow(t), Publisher: domainmocks.NewMockReportPublisher(t), OutputKey: testOutputKey}

	// No <reference> argument: cobra rejects before the publisher is called.
	require.Error(t, execReport(t, deps, "push"))
}

func TestReportHasSubcommands(t *testing.T) {
	deps := Deps{Workflow: domainmocks.NewMockWorkflow(t), Publisher: domainmocks.NewMockReportPublisher(t), OutputKey: testOutputKey}

	cmd := New(deps)

	got := map[string]bool{}
	for _, sub := range cmd.Commands() {
		got[sub.Name()] = true
	}

	for _, want := range []string{"view", "merge", "push", "pull"} {
		require.Truef(t, got[want], "report should have %q subcommand", want)
	}
}

func TestPulledScore(t *testing.T) {
	assert.Equal(t, "n/a (no mutations)", pulledScore(domain.MutationScore{}))
	assert.Equal(t, "100.00% (1 killed, 0 survived)", pulledScore(domain.MutationScore{Summary: rpt.Summary{Total: 1, Killed: 1}}))
}
