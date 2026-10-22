package config

// Dotted keys of every configuration value gooze reads.
const (
	KeyVersion                     = "version"
	KeyOutput                      = "output"
	KeyNoCache                     = "no-cache"
	KeyNoUpdateCheck               = "no-update-check"
	KeyPathsExclude                = "paths.exclude"
	KeyRunParallel                 = "run.parallel"
	KeyRunMutationTimeout          = "run.mutation_timeout"
	KeyRunCoverageProfile          = "run.coverage_profile"
	KeyRunDiff                     = "run.diff"
	KeyRunMutagens                 = "run.mutagens"
	KeyRunWorkspace                = "run.workspace"
	KeyMutagenComparisonEnumBounds = "mutagen.comparison.enum_bounds"
	KeyThresholdTotal              = "threshold.total"
	KeyThresholdAverage            = "threshold.average"
	KeyThresholdFile               = "threshold.file"
	KeyThresholdDiffTotal          = "threshold.diff.total"
	KeyThresholdDiffAverage        = "threshold.diff.average"
	KeyThresholdDiffFile           = "threshold.diff.file"
	KeyReportFormat                = "report.format"
	KeyLogFilename                 = "log.filename"
	KeyLogLevel                    = "log.level"
	KeyLogVerbose                  = "log.verbose"
	KeyLogMaxSize                  = "log.max_size"
	KeyLogMaxBackups               = "log.max_backups"
	KeyLogMaxAge                   = "log.max_age"
	KeyLogCompress                 = "log.compress"
)

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
