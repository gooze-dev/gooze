package cmd

import (
	"github.com/spf13/cobra"

	"github.com/mouse-blink/gooze/internal/domain"
	m "github.com/mouse-blink/gooze/internal/model"
)

// listCmd represents the list command.
var listCmd = newListCmd()
var listExcludeFlags []string

func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list [paths...]",
		Short: "List source files and mutation counts",
		Long:  listLongDescription,
		RunE: func(_ *cobra.Command, args []string) error {
			paths := parsePaths(args)
			useCache := !noCacheFlag

			return workflow.Estimate(domain.EstimateArgs{
				Paths:    paths,
				Exclude:  listExcludeFlags,
				UseCache: useCache,
				Reports:  m.Path(reportsOutputDirFlag),
			})
		},
	}
	cmd.Flags().StringArrayVarP(&listExcludeFlags, "exclude", "x", nil, "exclude files matching regex (can be repeated)")

	return cmd
}

func init() {
	rootCmd.AddCommand(listCmd)
}
