// Package cmd provides the root command and CLI setup for gooze.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gooze.dev/pkg/gooze/internal/adapter"
	"gooze.dev/pkg/gooze/internal/controller"
	"gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
)

var goFileAdapter adapter.GoFileAdapter
var soirceFSAdapter adapter.SourceFSAdapter
var reportStore adapter.ReportStore
var fsAdapter adapter.SourceFSAdapter
var testAdapter adapter.TestRunnerAdapter
var orchestrator domain.Orchestrator
var mutagen domain.Mutagen
var workflow domain.Workflow
var ui controller.UI

// reportsOutputDirFlag is a root-level flag shared by commands that read/write reports.
var reportsOutputDirFlag string

// noCacheFlag disables incremental caching when set.
var noCacheFlag bool

func init() {
	ui = controller.NewUI(rootCmd, controller.IsTTY(os.Stdout))
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
		ui,
		orchestrator,
		mutagen,
	)
}

const pathPatternsHelp = `Supports Go-style path patterns:
  - ./...          recursively scan current directory
  - ./pkg/...      recursively scan pkg directory
  - ./cmd ./pkg    scan multiple directories`

const rootLongDescription = `Gooze is a mutation testing tool for Go that helps you assess the quality
of your test suite by introducing small changes (mutations) to your code
and verifying that your tests catch them.

` + pathPatternsHelp

const runLongDescription = `Run mutation testing for the given paths (default: current module).

` + pathPatternsHelp

const listLongDescription = `List source files and the number of applicable mutations.

` + pathPatternsHelp

// rootCmd represents the base command when called without any subcommands.
var rootCmd = newRootCmd()

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gooze",
		Short: "Go mutation testing tool",
		Long:  rootLongDescription,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	cmd.PersistentFlags().StringVarP(&reportsOutputDirFlag, "output", "o", ".gooze-reports", "output directory for mutation testing reports")
	cmd.PersistentFlags().BoolVar(&noCacheFlag, "no-cache", false, "disable cached incremental runs (re-test everything)")

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

func parsePaths(args []string) []m.Path {
	paths := make([]m.Path, 0, len(args))
	for _, arg := range args {
		paths = append(paths, m.Path(arg))
	}

	return paths
}
