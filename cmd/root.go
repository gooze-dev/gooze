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

var listFlag bool

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

	// Get all sources from all paths
	wf := domain.NewWorkflow()

	sources, err := wf.GetSources(mPaths...)
	if err != nil {
		return fmt.Errorf("error processing paths: %w", err)
	}

	// Use factory to create appropriate UI based on TTY detection
	useTTY := adapter.IsTTY(cmd.OutOrStdout())
	ui := adapter.NewUI(cmd, useTTY)

	// Handle list flag - show mutation counts
	if listFlag {
		// Calculate estimations for all sources
		estimations := make(map[m.Path]adapter.MutationEstimation)

		for _, source := range sources {
			arithmeticCount, err := wf.EstimateMutations(source, m.MutationArithmetic)
			if err != nil {
				return fmt.Errorf("failed to estimate arithmetic mutations for %s: %w", source.Origin, err)
			}

			booleanCount, err := wf.EstimateMutations(source, m.MutationBoolean)
			if err != nil {
				return fmt.Errorf("failed to estimate boolean mutations for %s: %w", source.Origin, err)
			}

			estimations[source.Origin] = adapter.MutationEstimation{
				Arithmetic: arithmeticCount,
				Boolean:    booleanCount,
			}
		}

		return ui.DisplayMutationEstimations(estimations)
	}

	// Default behavior: run mutation testing
	return runMutationTests(wf, ui, sources)
}

// runMutationTests executes mutation testing on all sources.
func runMutationTests(wf domain.Workflow, ui adapter.UI, sources []m.Source) error {
	if len(sources) == 0 {
		return ui.ShowNotImplemented(0)
	}

	// Create a map to track reports per source file
	type FileResult struct {
		Source  m.Source
		Reports []m.Report
	}

	fileResults := make(map[m.Path]interface{})

	// Initialize all sources with empty reports
	for _, source := range sources {
		fileResults[source.Origin] = &FileResult{
			Source:  source,
			Reports: []m.Report{},
		}
	}

	for _, source := range sources {
		// Generate all mutations for this source (both arithmetic and boolean)
		mutations, err := wf.GenerateMutations(source) // No type specified = all types
		if err != nil {
			return fmt.Errorf("failed to generate mutations for %s: %w", source.Origin, err)
		}

		// Test each mutation
		for _, mutation := range mutations {
			report, err := wf.TestMutation(source, mutation)
			if err != nil {
				return fmt.Errorf("failed to test mutation %s: %w", mutation.ID, err)
			}

			value, ok := fileResults[source.Origin].(*FileResult)
			if !ok {
				return fmt.Errorf("unexpected file result type for %s", source.Origin)
			}

			value.Reports = append(value.Reports, report)
		}
	}

	// Display results
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
