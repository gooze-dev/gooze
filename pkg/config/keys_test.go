package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKeys(t *testing.T) {
	assert.Equal(t, []string{
		KeyVersion,
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
		KeyMutagenComparisonEnumBounds,
		KeyThresholdTotal,
		KeyThresholdAverage,
		KeyThresholdFile,
		KeyThresholdMaxSurvivors,
		KeyThresholdDiffTotal,
		KeyThresholdDiffAverage,
		KeyThresholdDiffFile,
		KeyReportFormat,
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
		"version", "output", "no-cache", "no-update-check", "paths.exclude",
		"run.parallel", "run.mutation_timeout", "run.coverage_profile", "run.diff", "run.mutagens",
		"run.workspace", "mutagen.comparison.enum_bounds",
		"threshold.total", "threshold.average", "threshold.file", "threshold.max_survivors",
		"threshold.diff.total", "threshold.diff.average", "threshold.diff.file",
		"report.format",
		"log.filename", "log.level", "log.verbose", "log.max_size",
		"log.max_backups", "log.max_age", "log.compress",
	}, Keys())
}
