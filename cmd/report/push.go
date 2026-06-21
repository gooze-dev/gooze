package report

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
)

func newPushCmd(deps Deps) *cobra.Command {
	return newTransferCmd(
		"push <reference>",
		"Push reports to an OCI registry",
		"Package the reports directory and push it to an OCI registry as an artifact.",
		func(ctx context.Context, ref string, opts registryFlags) error {
			return deps.Publisher.Push(ctx, domain.PushArgs{
				Reports:   m.Path(viper.GetString(deps.OutputKey)),
				Ref:       ref,
				PlainHTTP: opts.plainHTTP,
				Insecure:  opts.insecure,
			})
		},
	)
}
