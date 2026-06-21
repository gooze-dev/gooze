package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
)

var mutationTimeoutFlag int64
var runParallelFlag int
var runShardFlag string
var runEstimateFlag bool

// runCmd represents the run command.
var runCmd = newRunCmd()

func newRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run [paths...]",
		Short: "Run mutation testing",
		Long:  runLongDescription,
		RunE: func(_ *cobra.Command, args []string) error {
			// Default to scanning the current module recursively when no paths
			// are given (equivalent to `gooze run ./...`).
			if len(args) == 0 {
				args = []string{"./..."}
			}

			reportsPath := m.Path(viper.GetString(outputFlagName))
			estimateArgs := domain.EstimateArgs{
				Paths:    parsePaths(args),
				Exclude:  viper.GetStringSlice(excludeConfigKey),
				UseCache: !viper.GetBool(noCacheFlagName),
				Reports:  reportsPath,
			}

			if runEstimateFlag {
				return workflow.Estimate(context.Background(), estimateArgs)
			}

			shardIndex, totalShards := parseShardFlag(runShardFlag)
			timeoutSeconds := viper.GetInt64(mutationTimeoutKey)

			return workflow.Test(context.Background(), domain.TestArgs{
				EstimateArgs:    estimateArgs,
				Reports:         reportsPath,
				Threads:         viper.GetInt(runParallelConfigKey),
				ShardIndex:      shardIndex,
				TotalShardCount: totalShards,
				MutationTimeout: time.Duration(timeoutSeconds) * time.Second,
			})
		},
	}

	configureRunFlags(cmd)

	return cmd
}

func init() {
	rootCmd.AddCommand(runCmd)
}

func configureRunFlags(cmd *cobra.Command) {
	cmd.Flags().Int64Var(&mutationTimeoutFlag, mutationTimeoutFlagName, viper.GetInt64(mutationTimeoutKey), "timeout duration for testing a mutation in seconds")
	bindFlagToConfig(cmd.Flags().Lookup(mutationTimeoutFlagName), mutationTimeoutKey)

	cmd.Flags().IntVarP(&runParallelFlag, runParallelFlagName, "p", viper.GetInt(runParallelConfigKey), "number of parallel workers for mutation testing")
	bindFlagToConfig(cmd.Flags().Lookup(runParallelFlagName), runParallelConfigKey)
	cmd.Flags().StringVarP(&runShardFlag, "shard", "s", "", "shard index and total shard count in the format INDEX/TOTAL (e.g., 0/3)")
	cmd.Flags().BoolVar(&runEstimateFlag, "estimate", false, "list source files and applicable mutation counts without running tests")
}

func parseShardFlag(shard string) (int, int) {
	if shard == "" {
		return 0, 1
	}

	var index, total int

	_, err := fmt.Sscanf(shard, "%d/%d", &index, &total)
	if err != nil || total <= 0 || index < 0 || index >= total {
		return 0, 1
	}

	return index, total
}
