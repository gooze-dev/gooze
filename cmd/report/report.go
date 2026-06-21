// Package report provides the `gooze report` command group for working with
// mutation reports (view, merge, push, pull).
package report

import (
	"github.com/spf13/cobra"

	"gooze.dev/pkg/gooze/internal/domain"
)

// Deps are the dependencies the report commands need, supplied by the root
// command so this package stays decoupled from dependency wiring and config.
type Deps struct {
	Workflow  domain.Workflow
	Publisher domain.ReportPublisher
	OutputKey string // viper key holding the reports output directory
}

// New builds the `report` parent command and its subcommands.
func New(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Work with mutation reports",
		Long:  "View, merge, and move mutation testing reports between a registry and disk.",
		RunE: func(c *cobra.Command, _ []string) error {
			return c.Help()
		},
	}

	cmd.AddCommand(
		newViewCmd(deps),
		newMergeCmd(deps),
		newPushCmd(deps),
		newPullCmd(deps),
	)

	return cmd
}
