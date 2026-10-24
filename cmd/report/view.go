package report

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
)

const viewLongDescription = `View previously generated mutation reports from the reports directory (-o).

By default it prints one row per source file (mutations, killed, survived,
not covered, score), then every mutation that counts against the score as
"path:line: status (mutagen): original -> mutated", then the mutation score.

Paths limit the output to matching source files and read like ` + "`gooze run`" + `
arguments, relative to the module root:
  - ./...           every file (the default)
  - ./pkg/...       files under pkg, recursively
  - ./pkg           files directly in pkg
  - ./pkg/file.go   a single file

Use --all to list every mutation with its status and diff instead.

--format github writes a GitHub workflow annotation for each line with
surviving mutants before the summary, so they show on the pull request's
changed files; --format sarif writes a SARIF 2.1.0 log instead of the summary,
for code scanning; --format codeclimate (or gitlab) writes a Code Climate
JSON report instead of the summary, for GitLab Code Quality. Paths in all of
them are relative to the git repository root.
report.format in gooze.yaml sets the default.

Below a --threshold-* minimum, or with more surviving mutants than
--threshold-max-survivors, view exits with code 3. For the reports of a gooze
run --diff (view -o .gooze-reports/run), the --threshold-diff-* minimums apply
instead, as they do for the run, and only the survivors on the changed lines
count.`

func newViewCmd(deps Deps) *cobra.Command {
	var (
		all                        bool
		thresholds, diffThresholds ThresholdFlags
	)

	cmd := &cobra.Command{
		Use:   "view [paths...]",
		Short: "Summarize previously generated mutation reports",
		Long:  viewLongDescription,
		Example: `  gooze report view                      # per-file scores and survivors
  gooze report view ./internal/domain/... # only files under internal/domain
  gooze report view --all ./cmd/run.go    # every mutation of one file`,
		RunE: func(cmd *cobra.Command, args []string) error {
			paths := make([]m.Path, 0, len(args))
			for _, arg := range args {
				paths = append(paths, m.Path(arg))
			}

			minimums, diffMinimums, err := ReadGates(cmd, thresholds, diffThresholds)
			if err != nil {
				return err
			}

			view, err := viewFormatArgs(cmd, all)
			if err != nil {
				return err
			}

			view.Reports = m.Path(viper.GetString(deps.OutputKey))
			view.Paths = paths
			view.All = all
			view.Thresholds = minimums
			view.DiffThresholds = diffMinimums

			return deps.Workflow.View(cmd.Context(), view)
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "list every mutation with its status and diff instead of a per-file summary")
	addFormatFlag(cmd)
	thresholds = AddThresholdFlags(cmd, "threshold", "threshold")
	diffThresholds = AddThresholdFlags(cmd, "threshold.diff", "threshold-diff")

	return cmd
}

// viewFormatArgs returns the view's format settings. Text leaves them empty,
// so the view prints as it always has. --all lists mutations as text, so it
// ignores report.format and rejects --format.
func viewFormatArgs(cmd *cobra.Command, all bool) (domain.ViewArgs, error) {
	format, err := readFormat(cmd)
	if err != nil {
		return domain.ViewArgs{}, err
	}

	if !writesOutput[format] {
		return domain.ViewArgs{}, nil
	}

	if all {
		if cmd.Flags().Changed(formatFlagName) {
			return domain.ViewArgs{}, domain.WithExitCode(m.ExitUsage,
				fmt.Errorf("--all lists every mutation as text; it can't be combined with --format %s", format))
		}

		return domain.ViewArgs{}, nil
	}

	return domain.ViewArgs{
		Format:      format,
		Output:      cmd.OutOrStdout(),
		PathPrefix:  repoPathPrefix(cmd.Context()),
		ToolVersion: toolVersion(),
	}, nil
}
