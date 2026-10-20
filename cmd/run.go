package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
	"gooze.dev/pkg/gooze/pkg/config"
)

var mutationTimeoutFlag int64
var runParallelFlag int
var runShardFlag string
var runEstimateFlag bool
var runCoverageProfileFlag string

// runCmd represents the run command.
var runCmd = newRunCmd()

func newRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run [paths...]",
		Short: "Run mutation testing",
		Long:  runLongDescription,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateConfig(); err != nil {
				return err
			}

			// Default to scanning the current module recursively when no paths
			// are given (equivalent to `gooze run ./...`).
			if len(args) == 0 {
				args = []string{"./..."}
			}

			reportsPath := m.Path(viper.GetString(config.KeyOutput))
			estimateArgs := domain.EstimateArgs{
				Paths:    parsePaths(args),
				Exclude:  viper.GetStringSlice(config.KeyPathsExclude),
				UseCache: !viper.GetBool(config.KeyNoCache),
				Reports:  reportsPath,
			}

			if runEstimateFlag {
				return workflow.Estimate(cmd.Context(), estimateArgs)
			}

			shardIndex, totalShards, err := parseShardFlag(runShardFlag)
			if err != nil {
				return err
			}

			timeoutSeconds := viper.GetInt64(config.KeyRunMutationTimeout)

			return workflow.Test(cmd.Context(), domain.TestArgs{
				EstimateArgs:    estimateArgs,
				Reports:         reportsPath,
				Threads:         viper.GetInt(config.KeyRunParallel),
				ShardIndex:      shardIndex,
				TotalShardCount: totalShards,
				MutationTimeout: time.Duration(timeoutSeconds) * time.Second,
				CoverageProfile: m.Path(viper.GetString(config.KeyRunCoverageProfile)),
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
	cmd.Flags().Int64Var(&mutationTimeoutFlag, mutationTimeoutFlagName, viper.GetInt64(config.KeyRunMutationTimeout), "timeout duration for testing a mutation in seconds")
	bindFlagToConfig(cmd.Flags().Lookup(mutationTimeoutFlagName), config.KeyRunMutationTimeout)

	cmd.Flags().IntVarP(&runParallelFlag, runParallelFlagName, "p", viper.GetInt(config.KeyRunParallel), "number of parallel workers for mutation testing")
	bindFlagToConfig(cmd.Flags().Lookup(runParallelFlagName), config.KeyRunParallel)
	cmd.Flags().StringVarP(&runShardFlag, "shard", "s", "", "shard index and total shard count in the format INDEX/TOTAL (e.g., 0/3)")
	cmd.Flags().BoolVar(&runEstimateFlag, "estimate", false, "list source files and applicable mutation counts without running tests")

	cmd.Flags().StringVar(&runCoverageProfileFlag, coverageProfileFlagName, viper.GetString(config.KeyRunCoverageProfile), "path to a Go coverage profile; mutations on uncovered lines are reported as not_covered without running tests")
	bindFlagToConfig(cmd.Flags().Lookup(coverageProfileFlagName), config.KeyRunCoverageProfile)
}

// parseShardFlag parses INDEX/TOTAL; an empty value means no sharding.
func parseShardFlag(shard string) (int, int, error) {
	if shard == "" {
		return 0, 1, nil
	}

	var index, total int

	n, err := fmt.Sscanf(shard, "%d/%d", &index, &total)
	if err != nil || n != 2 || fmt.Sprintf("%d/%d", index, total) != strings.TrimSpace(shard) {
		return 0, 0, fmt.Errorf("invalid --shard %q: expected INDEX/TOTAL (e.g. 0/3)", shard)
	}

	if total <= 0 || index < 0 || index >= total {
		return 0, 0, fmt.Errorf("invalid --shard %q: index must satisfy 0 <= INDEX < TOTAL", shard)
	}

	return index, total, nil
}
