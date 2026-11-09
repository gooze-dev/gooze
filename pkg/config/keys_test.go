package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKeys(t *testing.T) {
	assert.Equal(t, []string{
		KeyOutput,
		KeyNoCache,
		KeyNoUpdateCheck,
		KeyPathsExclude,
		KeyRunParallel,
		KeyRunMutationTimeout,
		KeyRunCoverageProfile,
		KeyRunDiff,
		KeyRunMutagens,
		KeyRunWorkspace,
		KeyRunFailFast,
		KeyRunGoCache,
		KeyRunGoCacheSize,
		KeyRunGoCachePrewarm,
		KeyRunTestFlags,
		KeyMutagenComparisonEnumBounds,
		KeyThresholdTotal,
		KeyThresholdAverage,
		KeyThresholdFile,
		KeyThresholdMaxSurvivors,
		KeyThresholdTimeouts,
		KeyThresholdDiffTotal,
		KeyThresholdDiffAverage,
		KeyThresholdDiffFile,
		KeyThresholdDiffMaxSurvivors,
		KeyReportFormat,
		KeyReportGate,
		KeyReportOCICompat,
		KeyReportOCICompatHosts,
		KeyLogFilename,
		KeyLogLevel,
		KeyLogVerbose,
		KeyLogMaxSize,
		KeyLogMaxBackups,
		KeyLogMaxAge,
		KeyLogCompress,
	}, Keys())
}

func TestKeyValues(t *testing.T) {
	assert.Equal(t, []string{
		"output", "no_cache", "no_update_check", "paths.exclude",
		"run.parallel", "run.mutation_timeout", "run.coverage_profile", "run.diff", "run.mutagens",
		"run.workspace", "run.fail_fast", "run.go_cache", "run.go_cache_size", "run.go_cache_prewarm",
		"run.test_flags",
		"mutagen.comparison.enum_bounds",
		"threshold.total", "threshold.average", "threshold.file", "threshold.max_survivors", "threshold.timeouts",
		"threshold.diff.total", "threshold.diff.average", "threshold.diff.file", "threshold.diff.max_survivors",
		"report.format",
		"report.gate",
		"report.oci.compat",
		"report.oci.compat_hosts",
		"log.filename", "log.level", "log.verbose", "log.max_size",
		"log.max_backups", "log.max_age", "log.compress",
	}, Keys())
}

func TestDeprecatedKeys(t *testing.T) {
	deprecated := DeprecatedKeys()

	assert.Equal(t, map[string]string{"no-cache": "no_cache", "no-update-check": "no_update_check", "version": ""}, deprecated)

	for old, current := range deprecated {
		_, ok := Lookup(old)
		assert.False(t, ok, old)

		if current != "" {
			_, ok := Lookup(current)
			assert.True(t, ok, current)
		}
	}
}
