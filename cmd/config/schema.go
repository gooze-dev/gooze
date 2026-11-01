package config

import (
	"github.com/spf13/cobra"
	"gooze.dev/pkg/gooze/pkg/config"
)

func newSchemaCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "schema",
		Short: "Print the JSON Schema of gooze.yaml",
		Long: `Print the JSON Schema of gooze.yaml, for editors to validate and complete the
file with. Save it and point yaml-language-server at it, for example.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := cmd.OutOrStdout().Write(config.JSONSchema())
			return err
		},
	}
}
