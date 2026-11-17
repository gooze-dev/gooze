package report

import (
	"github.com/spf13/cobra"
	"gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
	"gooze.dev/pkg/gooze/pkg/config"
)

const maxPerFileFlagName = "max-per-file"

// addMaxPerFileFlag adds --max-per-file, whose default comes from
// report.max_per_file.
func addMaxPerFileFlag(cmd *cobra.Command) {
	cmd.Flags().Int(maxPerFileFlagName, 0,
		"list the survivors of at most N lines per file in the text summary and the github annotations, then count the rest (0: every line; config report.max_per_file)")
}

// readMaxPerFile returns the --max-per-file flag or else report.max_per_file.
func readMaxPerFile(cmd *cobra.Command) (int, error) {
	limit, err := readCount(cmd, maxPerFileFlagName, config.KeyReportMaxPerFile)
	if err != nil {
		return 0, domain.WithExitCode(m.ExitUsage, err)
	}

	return limit, nil
}
