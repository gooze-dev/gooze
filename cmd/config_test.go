package cmd

import (
	"maps"
	"slices"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
		"version":              1,
		"output":               ".gooze-reports",
		"no-cache":             false,
		"paths.exclude":        []string{},
		"run.parallel":         1,
		"run.mutation_timeout": 120,
		"run.coverage_profile": "",
		"log.filename":         ".gooze.log",
		"log.level":            0,
		"log.verbose":          false,
		"log.max_size":         10,
		"log.max_backups":      3,
		"log.max_age":          28,
		"log.compress":         true,
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
