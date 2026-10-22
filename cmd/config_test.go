package cmd

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	m "gooze.dev/pkg/gooze/internal/model"
	"gooze.dev/pkg/gooze/pkg/config"
)

func TestConfigConstants(t *testing.T) {
	assert.Equal(t, ".", configFolderPath)
	assert.Equal(t, "output", outputFlagName)
	assert.Equal(t, "no-cache", noCacheFlagName)
	assert.Equal(t, "exclude", excludeFlagName)
	assert.Equal(t, "parallel", runParallelFlagName)
	assert.Equal(t, "GOOZE", envPrefix)
}

func TestConfigFile(t *testing.T) {
	assert.Equal(t, "gooze.yaml", viper.ConfigFileUsed())
}

func TestConfigDefaults(t *testing.T) {
	want := map[string]any{
		"version":                1,
		"output":                 ".gooze-reports",
		"no-cache":               false,
		"no-update-check":        false,
		"paths.exclude":          []string{},
		"run.parallel":           1,
		"run.mutation_timeout":   120,
		"run.coverage_profile":   "",
		"run.diff":               "",
		"run.mutagens":           []string{},
		"run.workspace":          "overlay",
		"threshold.total":        0.0,
		"threshold.average":      0.0,
		"threshold.file":         0.0,
		"threshold.diff.total":   0.0,
		"threshold.diff.average": 0.0,
		"threshold.diff.file":    0.0,
		"log.filename":           ".gooze.log",
		"log.level":              "info",
		"log.verbose":            false,
		"log.max_size":           10,
		"log.max_backups":        3,
		"log.max_age":            28,
		"log.compress":           true,
	}

	assert.ElementsMatch(t, slices.Collect(maps.Keys(want)), viper.AllKeys())

	for key, value := range want {
		assert.Equal(t, value, viper.Get(key), key)
	}
}

func TestConfigSchemaCoversViperKeys(t *testing.T) {
	assert.ElementsMatch(t, config.Keys(), viper.AllKeys())

	for _, key := range viper.AllKeys() {
		field, ok := config.Lookup(key)
		require.True(t, ok, key)
		assert.Equal(t, field.Default, viper.Get(key), key)
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		key     string
		value   any
		wantErr string
	}{
		{key: config.KeyRunParallel, value: 4},
		{key: config.KeyRunParallel, value: 0, wantErr: "invalid run.parallel: 0 is out of range: must be at least 1"},
		{key: config.KeyRunMutationTimeout, value: 0, wantErr: "invalid run.mutation_timeout: 0 is out of range: must be at least 1"},
		{key: config.KeyVersion, value: 2, wantErr: "invalid version: 2 is out of range: must be 1"},
		{key: config.KeyThresholdTotal, value: 87.5},
		{key: config.KeyThresholdTotal, value: 100},
		{key: config.KeyThresholdFile, value: 100.5, wantErr: "invalid threshold.file: 100.5 is out of range: must be between 0 and 100"},
		{key: config.KeyThresholdDiffAverage, value: -1, wantErr: "invalid threshold.diff.average: -1 is out of range: must be between 0 and 100"},
		{key: config.KeyThresholdAverage, value: "high", wantErr: `invalid threshold.average: "high" is not a number`},
		{key: config.KeyRunWorkspace, value: "deep-copy"},
		{key: config.KeyRunWorkspace, value: "overly", wantErr: `invalid run.workspace: "overly" is not one of overlay, deep-copy`},
		{key: config.KeyLogLevel, value: "-4"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s=%v", tt.key, tt.value), func(t *testing.T) {
			// A default, unlike viper.Set, does not shadow flags in later tests.
			field, _ := config.Lookup(tt.key)
			viper.SetDefault(tt.key, tt.value)

			t.Cleanup(func() { viper.SetDefault(tt.key, field.Default) })

			err := validateConfig()
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}

			assert.EqualError(t, err, tt.wantErr)
			assert.Equal(t, m.ExitUsage, exitCode(context.Background(), err, false))
		})
	}
}

func TestValidateConfig_Defaults(t *testing.T) {
	assert.NoError(t, validateConfig())
}
