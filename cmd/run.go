package cmd

import (
	"github.com/spf13/cobra"

	"github.com/mouse-blink/gooze/internal/domain"
)

var runParallelFlag int
var runShardFlag string

// runCmd represents the run command.
var runCmd = newRunCmd()

func newRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run [paths...]",
		Short: "Run mutation testing",
		Long:  runLongDescription,
		RunE: func(_ *cobra.Command, args []string) error {
			shardIndex, totalShards := parseShardFlag(runShardFlag)
			paths := parsePaths(args)

			return workflow.Test(domain.TestArgs{
				EstimateArgs: domain.EstimateArgs{
					Paths:    paths,
					UseCache: false,
				},
				Reports:         ".gooze-reports",
				Threads:         runParallelFlag,
				ShardIndex:      shardIndex,
				TotalShardCount: totalShards,
			})
		},
	}
	cmd.Flags().IntVarP(&runParallelFlag, "parallel", "p", 1, "number of parallel workers for mutation testing")
	cmd.Flags().StringVarP(&runShardFlag, "shard", "s", "", "shard index and total shard count in the format INDEX/TOTAL (e.g., 0/3)")

	return cmd
}

func init() {
	rootCmd.AddCommand(runCmd)
}
