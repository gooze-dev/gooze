// Package cmd provides the root command and CLI setup for gooze.
package cmd

import (
	"fmt"
	"os"

	"github.com/mouse-blink/gooze/internal/adapter"
	"github.com/mouse-blink/gooze/internal/domain"
	m "github.com/mouse-blink/gooze/internal/model"
	"github.com/spf13/cobra"
)

var goFileAdapter adapter.GoFileAdapter
var soirceFSAdapter adapter.SourceFSAdapter
var reportStore adapter.ReportStore
var fsAdapter adapter.SourceFSAdapter
var testAdapter adapter.TestRunnerAdapter
var orchestrator domain.Orchestrator
var mutagen domain.Mutagen
var workflow domain.Workflow

func init() {
	goFileAdapter = adapter.NewLocalGoFileAdapter()
	soirceFSAdapter = adapter.NewLocalSourceFSAdapter()
	reportStore = adapter.NewReportStore()
	fsAdapter = adapter.NewLocalSourceFSAdapter()
	testAdapter = adapter.NewLocalTestRunnerAdapter()
	orchestrator = domain.NewOrchestrator(fsAdapter, testAdapter)
	mutagen = domain.NewMutagen(goFileAdapter, soirceFSAdapter)
	workflow = domain.NewWorkflow(
		soirceFSAdapter,
		reportStore,
		orchestrator,
		mutagen,
	)
}

var listFlag bool
var parallelFlag int
var shardFlag string

// rootCmd represents the base command when called without any subcommands.
var rootCmd = newRootCmd()

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gooze [paths...]",
		Short: "Go mutation testing tool",
		Long: `Gooze is a mutation testing tool for Go that helps you assess the quality
of your test suite by introducing small changes (mutations) to your code
and verifying that your tests catch them.

Supports Go-style path patterns:
  - ./...          recursively scan current directory
  - ./pkg/...      recursively scan pkg directory
  - ./cmd ./pkg    scan multiple directories`,
		RunE: func(_ *cobra.Command, args []string) error {
			shardIndex, totalShards := parseShardFlag(shardFlag)

			paths := make([]m.Path, 0, len(args))
			for _, arg := range args {
				paths = append(paths, m.Path(arg))
			}

			estimateArgs := domain.EstimateArgs{
				Paths:    paths,
				UseCache: listFlag,
			}
			if listFlag {
				return workflow.Estimate(estimateArgs)
			}

			return workflow.Test(domain.TestArgs{
				EstimateArgs:    estimateArgs,
				Reports:         ".gooze-reports",
				Threads:         parallelFlag,
				ShardIndex:      shardIndex,
				TotalShardCount: totalShards,
			})
		},
	}
	cmd.Flags().BoolVarP(&listFlag, "list", "l", false, "list all source files and count of mutations applicable")
	cmd.Flags().IntVarP(&parallelFlag, "parallel", "p", 1, "number of parallel workers for mutation testing")
	cmd.Flags().StringVarP(&shardFlag, "shard", "s", "", "shard index and total shard count in the format INDEX/TOTAL (e.g., 0/3)")

	return cmd
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
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
