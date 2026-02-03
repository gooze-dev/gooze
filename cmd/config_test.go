package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigConstants(t *testing.T) {
	assert.Equal(t, "gooze", configBaseName)
	assert.Equal(t, "gooze.yaml", configFileName)
	assert.Equal(t, ".", configFolderPath)
	assert.Equal(t, "output", outputFlagName)
	assert.Equal(t, "no-cache", noCacheFlagName)
	assert.Equal(t, "exclude", excludeFlagName)
	assert.Equal(t, "parallel", runParallelFlagName)
	assert.Equal(t, "run.parallel", runParallelConfigKey)
	assert.Equal(t, "paths.exclude", excludeConfigKey)
	assert.Equal(t, ".gooze-reports", defaultReportsDir)
	assert.Equal(t, false, defaultNoCache)
	assert.Equal(t, 1, defaultRunParallel)
	assert.Equal(t, "GOOZE", envPrefix)
}

func TestConfigVersionConstants(t *testing.T) {
	assert.Equal(t, "version", configVersionKey)
	assert.Equal(t, 1, currentConfigVersion)
}
