package config

// Dotted keys of every configuration value gooze reads.
const (
	KeyOutput                      = "output"
	KeyNoCache                     = "no_cache"
	KeyNoUpdateCheck               = "no_update_check"
	KeyPathsExclude                = "paths.exclude"
	KeyRunParallel                 = "run.parallel"
	KeyRunMutationTimeout          = "run.mutation_timeout"
	KeyRunCoverageProfile          = "run.coverage_profile"
	KeyRunDiff                     = "run.diff"
	KeyRunMutagens                 = "run.mutagens"
	KeyRunWorkspace                = "run.workspace"
	KeyRunFailFast                 = "run.fail_fast"
	KeyRunGoCache                  = "run.go_cache"
	KeyRunGoCacheSize              = "run.go_cache_size"
	KeyRunGoCachePrewarm           = "run.go_cache_prewarm"
	KeyMutagenComparisonEnumBounds = "mutagen.comparison.enum_bounds"
	KeyThresholdTotal              = "threshold.total"
	KeyThresholdAverage            = "threshold.average"
	KeyThresholdFile               = "threshold.file"
	KeyThresholdMaxSurvivors       = "threshold.max_survivors"
	KeyThresholdDiffTotal          = "threshold.diff.total"
	KeyThresholdDiffAverage        = "threshold.diff.average"
	KeyThresholdDiffFile           = "threshold.diff.file"
	KeyReportFormat                = "report.format"
	KeyReportOCICompat             = "report.oci.compat"
	KeyReportOCICompatHosts        = "report.oci.compat_hosts"
	KeyLogFilename                 = "log.filename"
	KeyLogLevel                    = "log.level"
	KeyLogVerbose                  = "log.verbose"
	KeyLogMaxSize                  = "log.max_size"
	KeyLogMaxBackups               = "log.max_backups"
	KeyLogMaxAge                   = "log.max_age"
	KeyLogCompress                 = "log.compress"
)

// KeyVersion named a config schema version that nothing read.
//
// Deprecated: gooze ignores the key; see [DeprecatedKeys].
const KeyVersion = "version"

// DeprecatedKeys returns the names gooze still accepts in the configuration
// file, each with the key that replaced it, or "" when it is ignored.
func DeprecatedKeys() map[string]string {
	return map[string]string{
		"no-cache":        KeyNoCache,
		"no-update-check": KeyNoUpdateCheck,
		KeyVersion:        "",
	}
}

// Keys returns the dotted key of every non-object field in schema order.
func Keys() []string {
	return appendKeys(nil, "", Schema())
}

func appendKeys(keys []string, prefix string, fields []Field) []string {
	for _, field := range fields {
		key := prefix + field.Name

		switch field.Kind {
		case KindObject:
			keys = appendKeys(keys, key+".", field.Children)
		case KindString, KindBool, KindInt, KindStringList, KindEnum, KindFloat:
			keys = append(keys, key)
		}
	}

	return keys
}
