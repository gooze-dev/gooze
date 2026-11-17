package cmd

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
	m "gooze.dev/pkg/gooze/internal/model"
)

// execRunWith runs the run command with args against wf and returns what it
// wrote to stdout.
func execRunWith(t *testing.T, wf domain.Workflow, args ...string) (string, error) {
	t.Helper()

	var out bytes.Buffer

	cmd := newRootCmd()
	cmd.AddCommand(newRunCmd())
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})

	originalWorkflow := workflow
	workflow = wf

	defer func() { workflow, runIgnoredFlag, runEstimateFlag = originalWorkflow, false, false }()

	cmd.SetArgs(append([]string{"run"}, args...))
	err := cmd.Execute()

	return out.String(), err
}

func TestRunCmd_IgnoredListsInsteadOfTesting(t *testing.T) {
	wf := domainmocks.NewMockWorkflow(t)
	wf.EXPECT().Ignored(mock.Anything, mock.MatchedBy(func(args domain.IgnoredArgs) bool {
		return len(args.Paths) == 1 && args.Paths[0] == m.Path("./pkg/...") && args.Output != nil
	})).Run(func(_ context.Context, args domain.IgnoredArgs) {
		_, _ = args.Output.Write([]byte("Ignored (1):\n"))
	}).Return(nil).Once()

	out, err := execRunWith(t, wf, "--ignored", "./pkg/...")

	require.NoError(t, err)
	assert.Equal(t, "Ignored (1):\n", out, "the list goes to stdout")
}

func TestRunCmd_IgnoredWithEstimateIsAUsageError(t *testing.T) {
	_, err := execRunWith(t, domainmocks.NewMockWorkflow(t), "--ignored", "--estimate")

	require.EqualError(t, err, "--ignored lists the ignored mutants; --estimate counts the tested ones: use one of them")
	assert.Equal(t, m.ExitUsage, exitCode(context.Background(), err, false))
}

func TestRunCmd_IgnoredFlagIsOffByDefault(t *testing.T) {
	flag := newRunCmd().Flags().Lookup("ignored")

	require.NotNil(t, flag)
	assert.Equal(t, "false", flag.DefValue)
}
