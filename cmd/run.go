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
var runDiffFlag string

// runPatternFlag (--run) and runMutagensFlag (--mutagens) narrow a run to
// some functions and mutagens. --run is a flag only, like go test -run.
var runPatternFlag string
var runMutagensFlag []string

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

			estimateArgs := runEstimateArgs(args)
			reportsPath := estimateArgs.Reports

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

	cmd.Flags().StringVar(&runDiffFlag, diffFlagName, viper.GetString(config.KeyRunDiff), "git ref to diff against; mutate only lines changed since its merge base with HEAD (requires git)")
	bindFlagToConfig(cmd.Flags().Lookup(diffFlagName), config.KeyRunDiff)

	cmd.Flags().StringVar(&runPatternFlag, runFlagName, "", "mutate only functions matching this regular expression, as for go test -run (matches Name, or Type.Name for methods)")

	cmd.Flags().StringSliceVar(&runMutagensFlag, mutagensFlagName, viper.GetStringSlice(config.KeyRunMutagens), "mutagens to run, comma-separated (default all: arithmetic, boolean, numbers, comparison, logical, unary)")
	bindFlagToConfig(cmd.Flags().Lookup(mutagensFlagName), config.KeyRunMutagens)
}

// runEstimateArgs builds the scan settings shared by --estimate and a run
// from the path arguments and config. No paths means ./..., the current
// module recursively.
func runEstimateArgs(args []string) domain.EstimateArgs {
	if len(args) == 0 {
		args = []string{"./..."}
	}

	return domain.EstimateArgs{
		Paths:    parsePaths(args),
		Exclude:  viper.GetStringSlice(config.KeyPathsExclude),
		UseCache: !viper.GetBool(config.KeyNoCache),
		Reports:  m.Path(viper.GetString(config.KeyOutput)),
		Diff:     viper.GetString(config.KeyRunDiff),
		Run:      runPatternFlag,
		Mutagens: viper.GetStringSlice(config.KeyRunMutagens),
	}
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
