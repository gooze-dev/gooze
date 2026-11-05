package report

import (
	"errors"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
)

const mergeLongDescription = `Merge reports from shard_* subdirectories into a single reports directory.

Each shard dir holds the results its run tested and copies of the stored
(cached) results of the unchanged sources it owns, so the merge gives the same
score as an unsharded run, also when only the shard dirs reach the merge job.

Then it shows the merged reports as report view does: the per-file table,
the survivors and the score, or less with --summary and --top, in --format.
Repeat --write <format>=<path> to write more reports from the same load, such
as --write gitlab=gl-code-quality.json --write metrics=metrics.txt.

Below a --threshold-* minimum, or with more surviving mutants than
--threshold-max-survivors, merge exits with code 3. For the shards of a gooze
run --diff (merge -o .gooze-reports/run), the --threshold-diff-* minimums
apply instead, as they do for report view. --gate (report.gate) picks the
scope whose thresholds decide the exit code: auto (the reports' own scope),
diff, total or none.`

func newMergeCmd(deps Deps) *cobra.Command {
	var (
		flags                      viewFlags
		thresholds, diffThresholds ThresholdFlags
	)

	cmd := &cobra.Command{
		Use:   "merge",
		Short: "Merge sharded reports into a single directory",
		Long:  mergeLongDescription,
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			minimums, diffMinimums, err := ReadGates(cmd, thresholds, diffThresholds)
			if err != nil {
				return err
			}

			view, closeWrites, err := flags.viewArgs(cmd)
			if err != nil {
				return err
			}

			reportsPath := m.Path(viper.GetString(deps.OutputKey))
			err = deps.Workflow.Merge(cmd.Context(), domain.MergeArgs{
				Reports: reportsPath, View: view, Thresholds: minimums, DiffThresholds: diffMinimums,
			})

			return errors.Join(err, closeWrites())
		},
	}

	flags.register(cmd)
	addFormatFlag(cmd)
	thresholds = AddThresholdFlags(cmd, "threshold", "threshold")
	diffThresholds = AddThresholdFlags(cmd, "threshold.diff", "threshold-diff")

	return cmd
}
