package config

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newInitCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Generate a default gooze.yaml configuration file",
		Long: `Create a gooze.yaml in the current working directory populated with the
current CLI defaults so it can be edited manually. An existing file is never
overwritten.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			targetPath := filepath.Join(deps.Dir, deps.FileName)

			if err := viper.SafeWriteConfigAs(targetPath); err != nil {
				var exists viper.ConfigFileAlreadyExistsError
				if errors.As(err, &exists) {
					return fmt.Errorf("%s already exists; edit it or remove it first", targetPath)
				}

				return fmt.Errorf("failed to write config file: %w", err)
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Wrote the default configuration to %s\n", targetPath)

			return nil
		},
	}
}
