package report

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
	"gooze.dev/pkg/gooze/pkg/config"
)

const writeFlagName = "write"

// writeFormats are the formats --write accepts: every --format but github,
// whose annotations only work in the job log.
func writeFormats() []string {
	field, _ := config.Lookup(config.KeyReportFormat)

	return slices.DeleteFunc(slices.Clone(field.Enum), func(format string) bool { return format == string(domain.FormatGitHub) })
}

// parseWrite splits a --write value, <format>=<path>, at its first =.
func parseWrite(value string) (domain.ViewFormat, string, error) {
	format, path, ok := strings.Cut(value, "=")
	if !ok || format == "" || path == "" {
		return "", "", domain.WithExitCode(m.ExitUsage, fmt.Errorf("--%s needs <format>=<path>, got %q", writeFlagName, value))
	}

	if !slices.Contains(writeFormats(), format) {
		return "", "", domain.WithExitCode(m.ExitUsage,
			fmt.Errorf("invalid --%s format %q: must be one of %s", writeFlagName, format, strings.Join(writeFormats(), ", ")))
	}

	return domain.ViewFormat(format), path, nil
}

// openWrites creates the file of each --write value and returns the writes
// and a function that closes the files. Every value is checked before a file
// is created, so a usage error leaves nothing behind.
func openWrites(values []string) ([]domain.ReportWrite, func() error, error) {
	if len(values) == 0 {
		return nil, func() error { return nil }, nil
	}

	formats := make([]domain.ViewFormat, len(values))
	paths := make([]string, len(values))

	for i, value := range values {
		format, path, err := parseWrite(value)
		if err != nil {
			return nil, nil, err
		}

		formats[i], paths[i] = format, path
	}

	files := make([]*os.File, 0, len(values))
	closeAll := func() error {
		var err error
		for _, file := range files {
			err = errors.Join(err, file.Close())
		}

		return err
	}

	writes := make([]domain.ReportWrite, 0, len(values))

	for i, path := range paths {
		file, err := os.Create(path) //nolint:gosec // a path the user asked to write.
		if err != nil {
			return nil, nil, errors.Join(fmt.Errorf("create the --%s file: %w", writeFlagName, err), closeAll())
		}

		files = append(files, file)
		writes = append(writes, domain.ReportWrite{Format: formats[i], Output: file})
	}

	return writes, closeAll, nil
}
