// Package cmd provides the root command and CLI setup for gooze.
package cmd

import (
	"fmt"
	"os"

	"github.com/mouse-blink/gooze/internal/adapter"
	"github.com/mouse-blink/gooze/internal/controller"
	"github.com/mouse-blink/gooze/internal/domain"
	m "github.com/mouse-blink/gooze/internal/model"
	"github.com/spf13/cobra"
)

var listFlag bool
var parallelFlag int

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
		RunE: runRoot,
	}

	cmd.Flags().BoolVarP(&listFlag, "list", "l", false, "list all source files and count of mutations applicable")
	cmd.Flags().IntVarP(&parallelFlag, "parallel", "p", 1, "number of parallel workers for mutation testing")

	return cmd
}

func runRoot(cmd *cobra.Command, args []string) error {
	// Default to current directory if no paths specified
	paths := args
	if len(paths) == 0 {
		paths = []string{"."}
	}

	// Convert string paths to m.Path type
	mPaths := make([]m.Path, len(paths))
	for i, p := range paths {
		mPaths[i] = m.Path(p)
	}

	// Create adapters
	fsAdapter := adapter.NewLocalSourceFSAdapter()
	goAdapter := adapter.NewLocalGoFileAdapter()
	testAdapter := adapter.NewLocalTestRunnerAdapter()

	// Get all sources from all paths
	wf := domain.NewWorkflow(fsAdapter, goAdapter, testAdapter)

	sources, err := wf.GetSources(mPaths...)
	if err != nil {
		return fmt.Errorf("error processing paths: %w", err)
	}

	// Use factory to create appropriate UI based on TTY detection
	useTTY := controller.IsTTY(cmd.OutOrStdout())
	ui := controller.NewUI(cmd, useTTY)

	// Handle list flag - show mutation counts
	if listFlag {
		// Calculate estimations for all sources
		estimations := make(map[m.Path]controller.MutationEstimation)

		for _, source := range sources {
			arithmeticCount, err := wf.EstimateMutations(source, m.MutationArithmetic)
			if err != nil {
				return fmt.Errorf("failed to estimate arithmetic mutations for %s: %w", source.Origin, err)
			}

			booleanCount, err := wf.EstimateMutations(source, m.MutationBoolean)
			if err != nil {
				return fmt.Errorf("failed to estimate boolean mutations for %s: %w", source.Origin, err)
			}

			estimations[source.Origin] = controller.MutationEstimation{
				Arithmetic: arithmeticCount,
				Boolean:    booleanCount,
			}
		}

		return ui.DisplayMutationEstimations(estimations)
	}

	// Default behavior: run mutation testing
	return runMutationTests(wf, ui, sources, parallelFlag)
}

// runMutationTests executes mutation testing on all sources.
func runMutationTests(wf domain.Workflow, ui controller.UI, sources []m.Source, threads int) error {
	if len(sources) == 0 {
		return ui.ShowNotImplemented(0)
	}

	// Delegate core mutation test execution to the workflow.
	results, err := wf.RunMutationTests(sources, threads)
	if err != nil {
		return err
	}

	// Adapt results to the UI's expected map type.
	fileResults := make(map[m.Path]interface{}, len(results))
	for path, res := range results {
		fileResults[path] = res
	}

	return ui.DisplayMutationResults(sources, fileResults)
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
