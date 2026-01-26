package cmd

import (
	"github.com/mouse-blink/gooze/internal/domain"
	m "github.com/mouse-blink/gooze/internal/model"
	"github.com/spf13/cobra"
)

// viewCmd represents the view command.
var viewCmd = newViewCmd()

func newViewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "view",
		Short: "View previously generated mutation reports",
		Long:  "View previously generated mutation reports from a reports directory.",
		Args:  cobra.ExactArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			return workflow.View(domain.ViewArgs{Reports: m.Path(reportsOutputDirFlag)})
		},
	}

	return cmd
}

func init() {
	rootCmd.AddCommand(viewCmd)
}
