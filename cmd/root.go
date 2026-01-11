// Package cmd provides the root command and CLI setup for gooze.
package cmd

import (
	"fmt"
	"os"

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

	cmd.Flags().BoolVarP(&listFlag, "list", "l", false, "list all source files and their mutation scopes")

	return cmd
}

func runRoot(cmd *cobra.Command, args []string) error {
	// Default to current directory if no paths specified
	paths := args
	if len(paths) == 0 {
		paths = []string{"."}
	}

	// Process each path
	var allSources []m.Source

	for _, path := range paths {
		sources, err := processPath(path)
		if err != nil {
			return fmt.Errorf("error processing %s: %w", path, err)
		}

		allSources = append(allSources, sources...)
	}

	// Handle list flag
	if listFlag {
		return listSources(cmd, allSources)
	}

	// TODO: implement mutation testing logic
	cmd.Println("Mutation testing not yet implemented. Use --list to see source files.")

	return nil
}

func processPath(path string) ([]m.Source, error) {
	wf := domain.NewWorkflow()
	// GetSources now handles both recursive (./...) and non-recursive patterns
	return wf.GetSources(m.Path(path))
}

func listSources(cmd *cobra.Command, sources []m.Source) error {
	if len(sources) == 0 {
		cmd.Println("No source files found")
		return nil
	}

	for _, source := range sources {
		cmd.Println(source.Origin)
	}

	return nil
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
