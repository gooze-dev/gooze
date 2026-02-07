package cmd

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
)

// mergeCmd represents the merge command.
var mergeCmd = newMergeCmd()

func newMergeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "merge",
		Short: "Merge sharded reports into a single directory",
		Long:  "Merge reports from shard_* subdirectories into a single reports directory.",
		Args:  cobra.ExactArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			reportsPath := m.Path(viper.GetString(outputFlagName))
			return workflow.Merge(context.Background(), domain.MergeArgs{Reports: reportsPath})
		},
	}

	return cmd
}

func init() {
	rootCmd.AddCommand(mergeCmd)
}
