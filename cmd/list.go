package cmd

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
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
			useCache := !viper.GetBool(noCacheFlagName)
			reportsPath := m.Path(viper.GetString(outputFlagName))

			return workflow.Estimate(context.Background(), domain.EstimateArgs{
				Paths:    paths,
				Exclude:  viper.GetStringSlice(excludeConfigKey),
				UseCache: useCache,
				Reports:  reportsPath,
			})
		},
	}

	return cmd
}

func init() {
	rootCmd.AddCommand(listCmd)
}
