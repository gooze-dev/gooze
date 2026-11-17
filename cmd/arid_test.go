package cmd

import (
	"bytes"
	"context"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	domain "gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
	m "gooze.dev/pkg/gooze/internal/model"
)

// aridOfRun returns the arid options a run with args hands the workflow.
func aridOfRun(t *testing.T, args ...string) domain.AridOptions {
	t.Helper()

	mockWorkflow := domainmocks.NewMockWorkflow(t)

	cmd := newRootCmd()
	cmd.AddCommand(newRunCmd())
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	originalWorkflow := workflow
	workflow = mockWorkflow
	defer func() { workflow = originalWorkflow }()
	// Rebind arid.enabled to a fresh, unset flag so later tests see the default.
	defer newRunCmd()

	var got domain.AridOptions

	mockWorkflow.On("Test", mock.Anything, mock.MatchedBy(func(args domain.TestArgs) bool {
		got = args.Arid

		return true
	})).Return(nil)

	cmd.SetArgs(append([]string{"run"}, args...))
	require.NoError(t, cmd.Execute())

	return got
}

func TestRunCmd_AridIsOffByDefault(t *testing.T) {
	assert.Equal(t, domain.AridOptions{Rules: []string{}, Calls: []string{}}, aridOfRun(t, "./..."))
	assert.Equal(t, "false", newRunCmd().Flags().Lookup(aridFlagName).DefValue)
}

func TestRunCmd_AridFlagAndConfig(t *testing.T) {
	assert.True(t, aridOfRun(t, "--arid", "./...").Enabled)

	t.Setenv("GOOZE_ARID_ENABLED", "true")
	assert.True(t, aridOfRun(t, "./...").Enabled)
	assert.False(t, aridOfRun(t, "--arid=false", "./...").Enabled, "the flag wins over the environment")

	require.NoError(t, viper.MergeConfigMap(map[string]any{"arid": map[string]any{
		"rules": []any{"logging"}, "calls": []any{"audit.*", "*.Tracef"},
	}}))
	t.Cleanup(func() {
		_ = viper.MergeConfigMap(map[string]any{"arid": map[string]any{
			"rules": []any{}, "calls": []any{},
		}})
	})

	assert.Equal(t, domain.AridOptions{Enabled: true, Rules: []string{"logging"}, Calls: []string{"audit.*", "*.Tracef"}}, aridOfRun(t, "./..."))
}

func TestRunCmd_RejectsUnknownAridRule(t *testing.T) {
	cmd := newRootCmd()
	cmd.AddCommand(newRunCmd())
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	// A mock without expectations: a run that gets past the check fails at once.
	originalWorkflow := workflow
	workflow = domainmocks.NewMockWorkflow(t)
	defer func() { workflow = originalWorkflow }()

	require.NoError(t, viper.MergeConfigMap(map[string]any{"arid": map[string]any{"rules": []any{"logging", "sleeping"}}}))
	t.Cleanup(func() {
		_ = viper.MergeConfigMap(map[string]any{"arid": map[string]any{"rules": []any{}}})
	})

	cmd.SetArgs([]string{"run", "./..."})
	err := cmd.Execute()
	require.ErrorContains(t, err, "invalid arid.rules")
	require.ErrorContains(t, err, "sleeping")
	assert.Equal(t, m.ExitUsage, exitCode(context.Background(), err, false))
}
