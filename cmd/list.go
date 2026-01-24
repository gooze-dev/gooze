package cmd

import (
	"github.com/spf13/cobra"

	"github.com/mouse-blink/gooze/internal/domain"
)

// listCmd represents the list command.
var listCmd = newListCmd()

func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list [paths...]",
		Short: "List source files and mutation counts",
		Long:  listLongDescription,
		RunE: func(_ *cobra.Command, args []string) error {
			paths := parsePaths(args)

			return workflow.Estimate(domain.EstimateArgs{
				Paths:    paths,
				UseCache: true,
			})
		},
	}

	return cmd
}

func init() {
	rootCmd.AddCommand(listCmd)
}
