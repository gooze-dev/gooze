// Package cmd provides the root command and CLI setup for gooze.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
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

// excludePatterns is a root-level flag that filters files for applicable commands.
var excludePatterns []string

func init() {
	configureRootFlags(rootCmd)

	// Initialize shared dependencies.
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
var rootCmd = baseRootCmd()

func baseRootCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "gooze",
		Short: "Go mutation testing tool",
		Long:  rootLongDescription,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
}

func configureRootFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().
		StringVarP(
			&reportsOutputDirFlag, outputFlagName, "o",
			viper.GetString(outputFlagName),
			"output directory for mutation testing reports",
		)
	bindFlagToConfig(cmd.PersistentFlags().Lookup(outputFlagName), outputFlagName)

	cmd.PersistentFlags().BoolVar(&noCacheFlag, noCacheFlagName, viper.GetBool(noCacheFlagName), "disable cached incremental runs (re-test everything)")
	bindFlagToConfig(cmd.PersistentFlags().Lookup(noCacheFlagName), noCacheFlagName)

	cmd.PersistentFlags().StringArrayVarP(&excludePatterns, excludeFlagName, "x", viper.GetStringSlice(excludeConfigKey), "exclude files matching regex (can be repeated)")
	bindFlagToConfig(cmd.PersistentFlags().Lookup(excludeFlagName), excludeConfigKey)
}

// bindFlagToConfig wires a Cobra flag to a Viper key so config/env values feed the flag.
func bindFlagToConfig(flag *pflag.Flag, key string) {
	if flag == nil {
		cobra.CheckErr(fmt.Errorf("flag for config key %q not found", key))
		return
	}

	cobra.CheckErr(viper.BindPFlag(key, flag))
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func parsePaths(args []string) []m.Path {
	paths := make([]m.Path, 0, len(args))
	for _, arg := range args {
		paths = append(paths, m.Path(arg))
	}

	return paths
}
