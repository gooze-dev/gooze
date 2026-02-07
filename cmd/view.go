package cmd

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
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
			reportsPath := m.Path(viper.GetString(outputFlagName))
			return workflow.View(context.Background(), domain.ViewArgs{Reports: reportsPath})
		},
	}

	return cmd
}

func init() {
	rootCmd.AddCommand(viewCmd)
}
