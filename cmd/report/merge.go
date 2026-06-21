package report

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
)

func newMergeCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "merge",
		Short: "Merge sharded reports into a single directory",
		Long:  "Merge reports from shard_* subdirectories into a single reports directory.",
		Args:  cobra.ExactArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			reportsPath := m.Path(viper.GetString(deps.OutputKey))
			return deps.Workflow.Merge(context.Background(), domain.MergeArgs{Reports: reportsPath})
		},
	}
}
