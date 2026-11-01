package cmd

import (
	"bytes"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
	"gooze.dev/pkg/gooze/pkg/config"
)

// executeRun runs gooze run against a mock workflow and returns the error and
// the log level the run saw.
func executeRun(t *testing.T, mockCalled bool, args ...string) (string, error) {
	t.Helper()

	mockWorkflow := domainmocks.NewMockWorkflow(t)
	if mockCalled {
		mockWorkflow.On("Test", mock.Anything, mock.Anything).Return(nil)
	}

	originalWorkflow := workflow
	workflow = mockWorkflow

	t.Cleanup(func() {
		workflow = originalWorkflow

		newRootCmd() // Rebind the keys to fresh, unset flags for later tests.
		newRunCmd()
	})

	cmd := newRootCmd()
	cmd.AddCommand(newRunCmd())
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(append([]string{"run"}, args...))

	err := cmd.Execute()

	return viper.GetString(config.KeyLogLevel), err
}

func TestLogLevelFlag_SetsTheLevel(t *testing.T) {
	level, err := executeRun(t, true, "--log-level", "warn", "./...")
	require.NoError(t, err)
	assert.Equal(t, "warn", level)
}

func TestLogLevelFlag_WinsOverTheEnvironment(t *testing.T) {
	t.Setenv("GOOZE_LOG_LEVEL", "error")

	level, err := executeRun(t, true, "--log-level", "debug", "./...")
	require.NoError(t, err)
	assert.Equal(t, "debug", level)
}

func TestLogLevelFlag_UnsetLeavesTheEnvironment(t *testing.T) {
	t.Setenv("GOOZE_LOG_LEVEL", "error")

	level, err := executeRun(t, true, "./...")
	require.NoError(t, err)
	assert.Equal(t, "error", level)
}

func TestLogLevelFlag_RejectsAnUnknownLevel(t *testing.T) {
	_, err := executeRun(t, false, "--log-level", "loud", "./...")
	require.ErrorContains(t, err, "invalid log.level")
}
