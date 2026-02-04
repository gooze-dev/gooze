package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
)

// DefaultMutationTimeout is the default timeout duration for testing a mutation.
const DefaultMutationTimeout = time.Minute * 2

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
			useCache := !viper.GetBool(noCacheFlagName)
			reportsPath := m.Path(viper.GetString(outputFlagName))
			threads := viper.GetInt(runParallelConfigKey)

			return workflow.Test(domain.TestArgs{
				EstimateArgs: domain.EstimateArgs{
					Paths:    paths,
					Exclude:  viper.GetStringSlice(excludeConfigKey),
					UseCache: useCache,
					Reports:  reportsPath,
				},
				Reports:         reportsPath,
				Threads:         threads,
				ShardIndex:      shardIndex,
				TotalShardCount: totalShards,
				MutationTimeout: DefaultMutationTimeout,
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
	cmd.Flags().IntVarP(&runParallelFlag, runParallelFlagName, "p", viper.GetInt(runParallelConfigKey), "number of parallel workers for mutation testing")
	bindFlagToConfig(cmd.Flags().Lookup(runParallelFlagName), runParallelConfigKey)
	cmd.Flags().StringVarP(&runShardFlag, "shard", "s", "", "shard index and total shard count in the format INDEX/TOTAL (e.g., 0/3)")
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
