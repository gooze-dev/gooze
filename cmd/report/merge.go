package report

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
)

func newMergeCmd(deps Deps) *cobra.Command {
	var thresholds ThresholdFlags

	cmd := &cobra.Command{
		Use:   "merge",
		Short: "Merge sharded reports into a single directory",
		Long: `Merge reports from shard_* subdirectories into a single reports directory.

The merged reports score the whole codebase, so the plain --threshold-*
minimums apply; the threshold.diff ones are for gooze run --diff and for
report view of its reports.`,
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			minimums, err := thresholds.Read(cmd)
			if err != nil {
				return err
			}

			reportsPath := m.Path(viper.GetString(deps.OutputKey))

			return deps.Workflow.Merge(cmd.Context(), domain.MergeArgs{Reports: reportsPath, Thresholds: minimums})
		},
	}

	thresholds = AddThresholdFlags(cmd, "threshold", "threshold")

	return cmd
}
