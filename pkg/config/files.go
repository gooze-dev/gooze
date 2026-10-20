package config

import "slices"

// FileName is the configuration file gooze loads from the working directory.
const FileName = "gooze.yaml"

// DefaultReportsDir is the directory gooze writes reports to by default.
const DefaultReportsDir = ".gooze-reports"

// FileNames returns the names of the configuration files gooze reads.
func FileNames() []string {
	return []string{FileName}
}

// IsConfigFile reports whether base, a file name without directory, is a file
// gooze reads its configuration from.
func IsConfigFile(base string) bool {
	return slices.Contains(FileNames(), base)
}
