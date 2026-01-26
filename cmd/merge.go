package cmd

import (
	"github.com/mouse-blink/gooze/internal/domain"
	m "github.com/mouse-blink/gooze/internal/model"
	"github.com/spf13/cobra"
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
			return workflow.Merge(domain.MergeArgs{Reports: m.Path(reportsOutputDirFlag)})
		},
	}

	return cmd
}

func init() {
	rootCmd.AddCommand(mergeCmd)
}
