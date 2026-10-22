package cmd

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
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
		{key: config.KeyRunMutationTimeout, value: "abc", wantErr: `invalid run.mutation_timeout: "abc" is not a whole number`},
		{key: config.KeyRunParallel, value: "8"},
		{key: config.KeyRunParallel, value: 1.5, wantErr: "invalid run.parallel: 1.5 is not a whole number"},
		{key: config.KeyRunParallel, value: -1.5, wantErr: "invalid run.parallel: -1.5 is not a whole number"},
		{key: config.KeyRunParallel, value: 2.0},
		{key: config.KeyRunParallel, value: true, wantErr: "invalid run.parallel: true is not a whole number"},
		{key: config.KeyLogLevel, value: "loud", wantErr: `invalid log.level: "loud" is not one of debug, info, warn, warning, error, or an integer`},
		{key: config.KeyLogLevel, value: " WARNING "},
		{key: config.KeyLogLevel, value: 4},
		{key: config.KeyLogLevel, value: ""},
		{key: config.KeyLogLevel, value: []any{"info"}, wantErr: "invalid log.level: a list is not one of debug, info, warn, warning, error, or an integer"},
		{key: config.KeyRunWorkspace, value: 7, wantErr: "invalid run.workspace: 7 is not one of overlay, deep-copy"},
		{key: config.KeyNoCache, value: "true"},
		{key: config.KeyNoCache, value: "maybe", wantErr: `invalid no-cache: "maybe" is not true or false`},
		{key: config.KeyOutput, value: 5},
		{key: config.KeyOutput, value: []any{"a"}, wantErr: "invalid output: a list is not a single value"},
		{key: config.KeyOutput, value: map[string]any{"a": 1}, wantErr: "invalid output: a mapping is not a single value"},
		{key: config.KeyPathsExclude, value: "^vendor/ ^mocks/"},
		{key: config.KeyPathsExclude, value: []string{"^vendor/"}},
		{key: config.KeyPathsExclude, value: []any{"^vendor/", 5}},
		{key: config.KeyPathsExclude, value: []any{[]any{"x"}}, wantErr: "invalid paths.exclude: item a list is not a single value"},
		{key: config.KeyPathsExclude, value: map[string]any{"a": "b"}, wantErr: "invalid paths.exclude: a mapping is not a list"},
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

func TestUnknownConfigKeys(t *testing.T) {
	assert.Empty(t, unknownConfigKeys(config.Keys()))
	assert.Equal(t, []string{
		`unknown gooze config key "bogus" in gooze.yaml`,
		`unknown gooze config key "logging.level" in gooze.yaml; did you mean "log.level"?`,
		`unknown gooze config key "run.paralel" in gooze.yaml; did you mean "run.parallel"?`,
	}, unknownConfigKeys(append([]string{"run.paralel", "bogus", "logging.level"}, config.Keys()...)))
}

func TestSuggestKey(t *testing.T) {
	tests := map[string]string{
		"parallel":         "run.parallel", // the only key with this last name
		"run.paralel":      "run.parallel", // one edit
		"run.parralel":     "run.parallel", // one edit
		"run.parlel":       "run.parallel", // two edits
		"run.prl":          "",             // too far
		"total":            "",             // threshold.total and threshold.diff.total
		"threshold.totl":   "threshold.total",
		"run.diffs":        "run.diff",
		"bogus":            "",
		"log.max_sizes":    "log.max_size",
		"threshold.diff.x": "", // threshold.diff.file is 3 edits away
		"log.max_sqe":      "", // 2 edits from both log.max_size and log.max_age
		"log.max_sizee":    "log.max_size",
		"log.max_xxx":      "",            // the closest, log.max_age, is 3 edits away
		"log.max_sge":      "log.max_age", // 1 edit; log.max_size, listed first, is 2
	}

	for key, want := range tests {
		assert.Equal(t, want, suggestKey(key), key)
	}
}

func TestEditDistance(t *testing.T) {
	assert.Equal(t, 0, editDistance("run.diff", "run.diff"))
	assert.Equal(t, 1, editDistance("run.paralel", "run.parallel"))
	assert.Equal(t, 1, editDistance("abc", "abd"))
	assert.Equal(t, 3, editDistance("", "abc"))
	assert.Equal(t, 3, editDistance("abc", ""))
	assert.Equal(t, 2, editDistance("ab", "ba"))
}

func TestRootCmd_RejectsBadValuesBeforeAnyCommand(t *testing.T) {
	mockWorkflow := domainmocks.NewMockWorkflow(t)

	cmd := newRootCmd()
	cmd.AddCommand(newRunCmd())
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	originalWorkflow := workflow
	workflow = mockWorkflow
	defer func() { workflow = originalWorkflow }()

	field, _ := config.Lookup(config.KeyLogLevel)
	viper.SetDefault(config.KeyLogLevel, "loud")
	t.Cleanup(func() { viper.SetDefault(config.KeyLogLevel, field.Default) })

	cmd.SetArgs([]string{"run", "./..."})
	err := cmd.Execute()

	require.EqualError(t, err, `invalid log.level: "loud" is not one of debug, info, warn, warning, error, or an integer`)
	assert.Equal(t, m.ExitUsage, exitCode(context.Background(), err, false))
}

func TestValidateConfig_Defaults(t *testing.T) {
	assert.NoError(t, validateConfig())
}

func TestSchemaDefault(t *testing.T) {
	t.Setenv("GOOZE_OUTPUT", "env-reports")

	assert.Equal(t, ".gooze-reports", schemaDefault[string](config.KeyOutput))
	assert.Equal(t, 120, schemaDefault[int](config.KeyRunMutationTimeout))
	assert.False(t, schemaDefault[bool](config.KeyNoCache))
	assert.Equal(t, []string{}, schemaDefault[[]string](config.KeyPathsExclude))
	assert.Equal(t, "overlay", schemaDefault[string](config.KeyRunWorkspace))
	assert.Empty(t, schemaDefault[string]("missing"))
}
