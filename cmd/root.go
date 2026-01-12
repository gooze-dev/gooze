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
		estimations := make(map[m.Path]int)

		for _, source := range sources {
			count, err := wf.EstimateMutations(source, m.MutationArithmetic)
			if err != nil {
				return fmt.Errorf("failed to estimate mutations for %s: %w", source.Origin, err)
			}

			estimations[source.Origin] = count
		}

		return ui.DisplayMutationEstimations(estimations)
	}

	// Without -l flag, show "not implemented" message
	return ui.ShowNotImplemented(len(sources))
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
