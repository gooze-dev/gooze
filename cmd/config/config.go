// Package config provides the `gooze config` command group for managing the
// gooze configuration file.
package config

import (
	"github.com/spf13/cobra"
)

// Deps are the dependencies the config commands need, supplied by the root
// command so this package stays decoupled from config-key wiring.
type Deps struct {
	Dir      string // directory the config file lives in (e.g. ".")
	FileName string // config file name (e.g. "gooze.yaml")
}

// New builds the `config` parent command and its subcommands.
func New(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage gooze configuration",
		Long:  "Generate and manage the gooze configuration file.",
		RunE: func(c *cobra.Command, _ []string) error {
			return c.Help()
		},
	}

	cmd.AddCommand(newInitCmd(deps))

	return cmd
}
