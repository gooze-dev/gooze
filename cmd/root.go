// Package cmd provides the root command and CLI setup for gooze.
package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"log/slog"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	configcmd "gooze.dev/pkg/gooze/cmd/config"
	"gooze.dev/pkg/gooze/cmd/report"
	"gooze.dev/pkg/gooze/internal/adapter"
	"gooze.dev/pkg/gooze/internal/controller"
	"gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
	"gooze.dev/pkg/gooze/pkg/config"
)

var goFileAdapter domain.GoFileAdapter
var sourceFSAdapter domain.SourceFSAdapter
var reportStore domain.ReportStore
var testAdapter domain.TestRunnerAdapter
var ociRegistry domain.OCIRegistry
var orchestrator domain.Orchestrator
var mutagen domain.Mutagen
var workflow domain.Workflow
var publisher domain.ReportPublisher
var ui controller.UI

// Logging flags.
var verboseFlag bool
var logOutputFlag string

// reportsOutputDirFlag is a root-level flag shared by commands that read/write reports.
var reportsOutputDirFlag string

// noCacheFlag disables incremental caching when set.
var noCacheFlag bool

// noUpdateCheckFlagValue disables the newer-release check when set.
var noUpdateCheckFlagValue bool

// excludePatterns is a root-level flag that filters files for applicable commands.
var excludePatterns []string

func init() {
	configureRootFlags(rootCmd)

	// Initialize shared dependencies.
	ui = controller.NewUI(rootCmd, controller.IsTTY(os.Stdout))
	goFileAdapter = adapter.NewLocalGoFileAdapter()
	sourceFSAdapter = adapter.NewLocalSourceFSAdapter()
	reportStore = adapter.NewReportStore()
	testAdapter = adapter.NewLocalTestRunnerAdapter()
	ociRegistry = adapter.NewORASRegistry()
	orchestrator = domain.NewOrchestrator(sourceFSAdapter, testAdapter)
	mutagen = domain.NewMutagen(goFileAdapter, sourceFSAdapter, adapter.NewUnifiedDiffer())
	workflow = domain.NewWorkflow(
		sourceFSAdapter,
		reportStore,
		ui,
		orchestrator,
		mutagen,
	)
	publisher = domain.NewReportPublisher(ociRegistry)
	releaseChecker = adapter.NewModuleProxyReleaseChecker()

	rootCmd.AddCommand(report.New(report.Deps{
		Workflow:  workflow,
		Publisher: publisher,
		OutputKey: config.KeyOutput,
	}))

	rootCmd.AddCommand(configcmd.New(configcmd.Deps{
		Dir:      configFolderPath,
		FileName: config.FileName,
	}))
}

const pathPatternsHelp = `Supports Go-style path patterns:
  - ./...          recursively scan current directory
  - ./pkg/...      recursively scan pkg directory
  - ./cmd ./pkg    scan multiple directories`

const rootLongDescription = `Gooze is a mutation testing tool for Go that helps you assess the quality
of your test suite by introducing small changes (mutations) to your code
and verifying that your tests catch them.

` + pathPatternsHelp

const runLongDescription = `Run mutation testing for the given paths. With no paths, defaults to ./...
(the current module, recursively).

Use --estimate to list source files and applicable mutation counts without
running tests.

` + pathPatternsHelp

// rootCmd represents the base command when called without any subcommands.
var rootCmd = baseRootCmd()

func baseRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gooze",
		Short: "Go mutation testing tool",
		Long:  rootLongDescription,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			// Flags and arguments are valid by now: a later error is a runtime
			// failure, so print only the error, not the usage.
			cmd.Root().SilenceUsage = true

			logPath := strings.TrimSpace(logOutputFlag)
			if logPath == "" {
				logPath = viper.GetString(config.KeyLogFilename)
			}

			if strings.TrimSpace(logPath) == "" {
				logPath = defaultLogFilename()
			}

			verbose := verboseFlag
			if !verbose {
				verbose = viper.GetBool(config.KeyLogVerbose)
			}

			configureLogger(logPath, verbose)
			startUpdateCheck(cmd)

			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			slog.Info("No command specified. Use --help to see available commands.")
			return cmd.Help()
		},
	}

	return cmd
}

func configureRootFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().
		StringVarP(
			&reportsOutputDirFlag, outputFlagName, "o",
			viper.GetString(config.KeyOutput),
			"output directory for mutation testing reports",
		)
	bindFlagToConfig(cmd.PersistentFlags().Lookup(outputFlagName), config.KeyOutput)

	cmd.PersistentFlags().BoolVar(&noCacheFlag, noCacheFlagName, viper.GetBool(config.KeyNoCache), "disable cached incremental runs (re-test everything)")
	bindFlagToConfig(cmd.PersistentFlags().Lookup(noCacheFlagName), config.KeyNoCache)

	cmd.PersistentFlags().BoolVar(&noUpdateCheckFlagValue, noUpdateCheckFlag, viper.GetBool(config.KeyNoUpdateCheck), "skip checking for a newer gooze release")
	bindFlagToConfig(cmd.PersistentFlags().Lookup(noUpdateCheckFlag), config.KeyNoUpdateCheck)

	cmd.PersistentFlags().StringArrayVarP(&excludePatterns, excludeFlagName, "x", viper.GetStringSlice(config.KeyPathsExclude), "exclude files matching regex (can be repeated)")
	bindFlagToConfig(cmd.PersistentFlags().Lookup(excludeFlagName), config.KeyPathsExclude)

	// Add verbose and log output flags (handled directly; not bound to Viper).
	cmd.PersistentFlags().BoolVarP(&verboseFlag, "verbose", "v", false, "enable verbose logging")
	cmd.PersistentFlags().StringVar(&logOutputFlag, "log-output", "", "path to the log output file")
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := rootCmd.ExecuteContext(ctx)

	printUpdateNotice(ctx, rootCmd.ErrOrStderr())
	stop()

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
