package cmd

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gooze.dev/pkg/gooze/pkg/config"
)

func TestParseSlogLevel(t *testing.T) {
	tests := []struct {
		value string
		want  slog.Level
	}{
		{value: "", want: slog.LevelWarn},
		{value: "debug", want: slog.LevelDebug},
		{value: " INFO ", want: slog.LevelInfo},
		{value: "warn", want: slog.LevelWarn},
		{value: "warning", want: slog.LevelWarn},
		{value: "error", want: slog.LevelError},
		{value: "-4", want: slog.Level(-4)},
		{value: "8", want: slog.Level(8)},
		{value: "loud", want: slog.LevelWarn},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			assert.Equal(t, tt.want, parseSlogLevel(tt.value, slog.LevelWarn))
		})
	}
}

// logInto configures the logger with logPath and returns what one info line
// wrote to want, restoring the global logger and the log.filename setting.
func logInto(t *testing.T, logPath, configured, want string) string {
	t.Helper()

	t.Chdir(t.TempDir())

	// A default, unlike viper.Set, does not shadow flags in later tests.
	field, _ := config.Lookup(config.KeyLogFilename)
	defaultLogger := slog.Default()
	t.Cleanup(func() {
		slog.SetDefault(defaultLogger)
		viper.SetDefault(config.KeyLogFilename, field.Default)
	})

	viper.SetDefault(config.KeyLogFilename, configured)
	configureLogger(logPath, false)
	slog.Info("logged here")

	content, err := os.ReadFile(want)
	require.NoError(t, err, "log written to %s", want)

	return string(content)
}

func TestConfigureLogger_WritesToTheGivenPath(t *testing.T) {
	content := logInto(t, "given.log", "configured.log", "given.log")

	assert.Contains(t, content, "logged here")
	assert.Contains(t, content, "source=", "log lines name their source")
	assert.NoFileExists(t, "configured.log")
}

func TestConfigureLogger_FallsBackToTheConfiguredPath(t *testing.T) {
	assert.Contains(t, logInto(t, "  ", "configured.log", "configured.log"), "logged here")
}

func TestConfigureLogger_FallsBackToTheDefaultPath(t *testing.T) {
	assert.Contains(t, logInto(t, "", " ", ".gooze.log"), "logged here")
}

// withConfigFile points viper at path for one test.
func withConfigFile(t *testing.T, path string) {
	t.Helper()

	previous := viper.ConfigFileUsed()
	viper.SetConfigFile(path)
	t.Cleanup(func() { viper.SetConfigFile(previous) })
}

// readConfigWarnings reads the config file at path and returns what it logged.
func readConfigWarnings(t *testing.T, path string) string {
	t.Helper()

	withConfigFile(t, path)

	var logged bytes.Buffer
	defaultLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
	t.Cleanup(func() { slog.SetDefault(defaultLogger) })

	readConfigFile()

	return logged.String()
}

func TestReadConfigFile(t *testing.T) {
	dir := t.TempDir()

	t.Run("missing file", func(t *testing.T) {
		assert.Empty(t, readConfigWarnings(t, filepath.Join(dir, "missing.yaml")))
	})

	t.Run("invalid yaml", func(t *testing.T) {
		path := filepath.Join(dir, "broken.yaml")
		require.NoError(t, os.WriteFile(path, []byte("paths: [\n"), 0o600))

		assert.Contains(t, readConfigWarnings(t, path), "failed to read config file")
	})

	t.Run("valid file", func(t *testing.T) {
		// No settings, so the values it loads don't shadow later tests' defaults.
		path := filepath.Join(dir, "valid.yaml")
		require.NoError(t, os.WriteFile(path, []byte("# no settings\n"), 0o600))

		assert.Empty(t, readConfigWarnings(t, path))
	})
}
