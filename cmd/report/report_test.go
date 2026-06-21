package report

import (
	"bytes"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
	m "gooze.dev/pkg/gooze/internal/model"
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
	wf.On("View", mock.Anything, domain.ViewArgs{Reports: m.Path("/tmp/reports")}).Return(nil)

	deps := Deps{Workflow: wf, Publisher: domainmocks.NewMockReportPublisher(t), OutputKey: testOutputKey}

	require.NoError(t, execReport(t, deps, "view"))
	wf.AssertExpectations(t)
}

func TestReportMerge(t *testing.T) {
	viper.Set(testOutputKey, "/tmp/reports")
	defer viper.Reset()

	wf := domainmocks.NewMockWorkflow(t)
	wf.On("Merge", mock.Anything, domain.MergeArgs{Reports: m.Path("/tmp/reports")}).Return(nil)

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
			args.Insecure
	})).Return(nil)

	deps := Deps{Workflow: domainmocks.NewMockWorkflow(t), Publisher: pub, OutputKey: testOutputKey}

	require.NoError(t, execReport(t, deps, "pull", "--insecure", "ghcr.io/org/repo:tag"))
	pub.AssertExpectations(t)
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
