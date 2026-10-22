package cmd

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"gooze.dev/pkg/gooze/cmd/report"
	"gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
	"gooze.dev/pkg/gooze/pkg/config"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
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
var runWorkspaceFlag string

// errRunWithShard rejects --run with --shard: a run of a few functions is too
// small to split across jobs.
var errRunWithShard = errors.New("--run can't be combined with --shard: a function-level run is small, so run it in one job")

// runCmd represents the run command.
var runCmd = newRunCmd()

func newRunCmd() *cobra.Command {
	var thresholds, diffThresholds report.ThresholdFlags

	cmd := &cobra.Command{
		Use:   "run [paths...]",
		Short: "Run mutation testing",
		Long:  runLongDescription,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateConfig(); err != nil {
				return err
			}

			estimateArgs := runEstimateArgs(args)

			if runEstimateFlag {
				return workflow.Estimate(cmd.Context(), estimateArgs)
			}

			shardIndex, totalShards, err := parseShardFlag(runShardFlag)
			if err != nil {
				return err
			}

			if totalShards > 1 && estimateArgs.Run != "" {
				return usageError(errRunWithShard)
			}

			minimums, err := runThresholds(cmd, estimateArgs.Diff != "", thresholds, diffThresholds)
			if err != nil {
				return err
			}

			return workflow.Test(cmd.Context(), runTestArgs(estimateArgs, shardIndex, totalShards, minimums))
		},
	}

	thresholds, diffThresholds = configureRunFlags(cmd)

	return cmd
}

// runTestArgs builds the arguments of a test run from the parsed flags and
// the config.
func runTestArgs(estimateArgs domain.EstimateArgs, shardIndex, totalShards int, minimums domain.Thresholds) domain.TestArgs {
	return domain.TestArgs{
		EstimateArgs:    estimateArgs,
		Reports:         estimateArgs.Reports,
		Threads:         viper.GetInt(config.KeyRunParallel),
		ShardIndex:      shardIndex,
		TotalShardCount: totalShards,
		MutationTimeout: time.Duration(viper.GetInt64(config.KeyRunMutationTimeout)) * time.Second,
		CoverageProfile: m.Path(viper.GetString(config.KeyRunCoverageProfile)),
		Workspace:       domain.WorkspaceMode(viper.GetString(config.KeyRunWorkspace)),
		Thresholds:      minimums,
	}
}

func init() {
	rootCmd.AddCommand(runCmd)
}

// configureRunFlags adds run's flags and returns its threshold flags: the
// plain ones, and the ones for --diff runs.
func configureRunFlags(cmd *cobra.Command) (report.ThresholdFlags, report.ThresholdFlags) {
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

	cmd.Flags().StringSliceVar(&runMutagensFlag, mutagensFlagName, viper.GetStringSlice(config.KeyRunMutagens), "mutagens to run, comma-separated: "+strings.Join(mgen.Names(), ", ")+" (default all but branch, statement and loop)")
	bindFlagToConfig(cmd.Flags().Lookup(mutagensFlagName), config.KeyRunMutagens)

	cmd.Flags().StringVar(&runWorkspaceFlag, workspaceFlagName, viper.GetString(config.KeyRunWorkspace), "how mutants are built: overlay (go test -overlay in the project) or deep-copy (a copy of the project per worker, for tests that build the code in a subprocess or read source files)")
	bindFlagToConfig(cmd.Flags().Lookup(workspaceFlagName), config.KeyRunWorkspace)

	return report.AddThresholdFlags(cmd, "threshold", "threshold"),
		report.AddThresholdFlags(cmd, "threshold.diff", "threshold-diff")
}

// runThresholds returns the minimum scores of a run: a --diff run is scored
// on its changed lines, so it has its own. Both sets are checked, so a bad
// value fails whichever applies.
func runThresholds(cmd *cobra.Command, diff bool, plain, diffOnly report.ThresholdFlags) (domain.Thresholds, error) {
	minimums, err := plain.Read(cmd)
	if err != nil {
		return domain.Thresholds{}, err
	}

	diffMinimums, err := diffOnly.Read(cmd)
	if err != nil {
		return domain.Thresholds{}, err
	}

	if diff {
		return diffMinimums, nil
	}

	return minimums, nil
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
		// Equivalent: callers ignore the numbers when err is set.
		return 0, 0, usageError(fmt.Errorf("invalid --shard %q: expected INDEX/TOTAL (e.g. 0/3)", shard)) //gooze:ignore numbers
	}

	if total <= 0 || index < 0 || index >= total {
		// Equivalent: callers ignore the numbers when err is set.
		return 0, 0, usageError(fmt.Errorf("invalid --shard %q: index must satisfy 0 <= INDEX < TOTAL", shard)) //gooze:ignore numbers
	}

	return index, total, nil
}
