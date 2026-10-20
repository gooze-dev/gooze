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
		KeyPathsExclude,
		KeyRunParallel,
		KeyRunMutationTimeout,
		KeyRunCoverageProfile,
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
		"version", "output", "no-cache", "paths.exclude",
		"run.parallel", "run.mutation_timeout", "run.coverage_profile",
		"log.filename", "log.level", "log.verbose", "log.max_size",
		"log.max_backups", "log.max_age", "log.compress",
	}, Keys())
}
