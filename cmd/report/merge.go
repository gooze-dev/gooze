package report

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
)

func newMergeCmd(deps Deps) *cobra.Command {
	var thresholds, diffThresholds ThresholdFlags

	cmd := &cobra.Command{
		Use:   "merge",
		Short: "Merge sharded reports into a single directory",
		Long: `Merge reports from shard_* subdirectories into a single reports directory.

Below a --threshold-* minimum, or with more surviving mutants than
--threshold-max-survivors, merge exits with code 3. For the shards of a gooze
run --diff (merge -o .gooze-reports/run), the --threshold-diff-* minimums
apply instead, as they do for report view.`,
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			minimums, diffMinimums, err := ReadGates(cmd, thresholds, diffThresholds)
			if err != nil {
				return err
			}

			reportsPath := m.Path(viper.GetString(deps.OutputKey))

			return deps.Workflow.Merge(cmd.Context(), domain.MergeArgs{Reports: reportsPath, Thresholds: minimums, DiffThresholds: diffMinimums})
		},
	}

	thresholds = AddThresholdFlags(cmd, "threshold", "threshold")
	diffThresholds = AddThresholdFlags(cmd, "threshold.diff", "threshold-diff")

	return cmd
}
