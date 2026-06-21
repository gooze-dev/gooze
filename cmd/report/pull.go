package report

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
)

func newPullCmd(deps Deps) *cobra.Command {
	return newTransferCmd(
		"pull <reference>",
		"Pull reports from an OCI registry",
		"Pull a reports artifact from an OCI registry and extract it into the reports directory.",
		func(ctx context.Context, ref string, opts registryFlags) error {
			return deps.Publisher.Pull(ctx, domain.PullArgs{
				Reports:   m.Path(viper.GetString(deps.OutputKey)),
				Ref:       ref,
				PlainHTTP: opts.plainHTTP,
				Insecure:  opts.insecure,
			})
		},
	)
}
