package cmd

import (
	"github.com/spf13/cobra"

	"gooze.dev/pkg/gooze/internal/adapter/gocache"
)

// newCacheProgCmd returns the hidden command the go command starts as
// GOCACHEPROG while gooze tests mutants. Its own PersistentPreRunE skips
// root's logging, config checks and update check.
func newCacheProgCmd() *cobra.Command {
	var shared, layer string

	cmd := &cobra.Command{
		Use:               "__cacheprog",
		Hidden:            true,
		Args:              cobra.NoArgs,
		PersistentPreRunE: func(*cobra.Command, []string) error { return nil },
		RunE: func(cmd *cobra.Command, _ []string) error {
			return gocache.Serve(cmd.InOrStdin(), cmd.OutOrStdout(), shared, layer)
		},
	}

	cmd.Flags().StringVar(&shared, "shared", "", "the shared GOCACHE, read only")
	cmd.Flags().StringVar(&layer, "layer", "", "gooze's own cache")

	return cmd
}
