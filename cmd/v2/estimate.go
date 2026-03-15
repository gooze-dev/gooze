package v2

import (
	"github.com/spf13/cobra"
	c "gooze.dev/pkg/gooze/internal/controller/v2"
)

const listLongDescription = `List source files and the number of applicable mutations.

` + pathPatternsHelp

var estimateCmd = &cobra.Command{
	Use:   "list [path patterns]",
	Short: "Estimate the number of mutations for the given paths",
	Long:  listLongDescription,
	RunE:  c.Estimate,
}

func init() {
	rootCmd.AddCommand(estimateCmd)
}
