package report

import (
	"context"

	"github.com/spf13/cobra"
)

// registryFlags holds the OCI registry connection flags shared by push and pull.
type registryFlags struct {
	plainHTTP bool
	insecure  bool
}

func (f *registryFlags) bind(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&f.plainHTTP, "plain-http", false, "use plain HTTP instead of HTTPS")
	cmd.Flags().BoolVar(&f.insecure, "insecure", false, "skip TLS certificate verification")
}

// newTransferCmd builds a push/pull command that takes a single <reference>
// argument and the shared registry flags, delegating to run.
func newTransferCmd(use, short, long string, run func(ctx context.Context, ref string, opts registryFlags) error) *cobra.Command {
	var opts registryFlags

	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Long:  long,
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return run(context.Background(), args[0], opts)
		},
	}

	opts.bind(cmd)

	return cmd
}
