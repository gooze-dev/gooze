package cmd

import (
	"github.com/spf13/cobra"

	"gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
)

var runParallelFlag int
var runShardFlag string
var runExcludeFlags []string

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
			useCache := !noCacheFlag

			return workflow.Test(domain.TestArgs{
				EstimateArgs: domain.EstimateArgs{
					Paths:    paths,
					Exclude:  runExcludeFlags,
					UseCache: useCache,
					Reports:  m.Path(reportsOutputDirFlag),
				},
				Reports:         m.Path(reportsOutputDirFlag),
				Threads:         runParallelFlag,
				ShardIndex:      shardIndex,
				TotalShardCount: totalShards,
			})
		},
	}
	cmd.Flags().IntVarP(&runParallelFlag, "parallel", "p", 1, "number of parallel workers for mutation testing")
	cmd.Flags().StringVarP(&runShardFlag, "shard", "s", "", "shard index and total shard count in the format INDEX/TOTAL (e.g., 0/3)")
	cmd.Flags().StringArrayVarP(&runExcludeFlags, "exclude", "x", nil, "exclude files matching regex (can be repeated)")

	return cmd
}

func init() {
	rootCmd.AddCommand(runCmd)
}
