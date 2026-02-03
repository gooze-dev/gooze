package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// initCmd represents the init command.
var initCmd = newInitCmd()

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Generate a default gooze.yaml configuration file",
		Long: `Create a gooze.yaml in the current working directory populated with the
current CLI defaults so it can be edited manually.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			targetPath := filepath.Join(configFolderPath, configFileName)

			err := viper.SafeWriteConfigAs(targetPath)
			if err != nil {
				return fmt.Errorf("failed to write config file: %w", err)
			}

			return nil
		},
	}
}

func init() {
	rootCmd.AddCommand(initCmd)
}
