package v2

import (
	"os"

	"github.com/spf13/cobra"
	c "gooze.dev/pkg/gooze/internal/controller/v2"
)

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



// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:              "gooze",
	Short:            "A tool for optimizing Go test execution",
	Long:             rootLongDescription,
	PersistentPreRun: c.Initialize,
	RunE:             c.Root,
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
