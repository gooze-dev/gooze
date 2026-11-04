package report

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
	"gooze.dev/pkg/gooze/pkg/config"
)

const ociCompatFlagName = "oci-compat"

func newPushCmd(deps Deps) *cobra.Command {
	var cmd *cobra.Command

	cmd = newTransferCmd(
		"push <reference>",
		"Push reports to an OCI registry",
		"Package the reports directory and push it to an OCI registry as an artifact.\n\n"+
			"Registries that accept only allowlisted media types, such as GitLab's, need compat media types:\n"+
			"--oci-compat (config report.oci.compat) turns them on, and they are on by themselves for\n"+
			"registry.gitlab.com and the hosts in report.oci.compat_hosts.",
		func(ctx context.Context, ref string, opts registryFlags) error {
			return deps.Publisher.Push(ctx, domain.PushArgs{
				Reports:        m.Path(viper.GetString(deps.OutputKey)),
				Ref:            ref,
				PlainHTTP:      opts.plainHTTP,
				Insecure:       opts.insecure,
				OCICompat:      readOCICompat(cmd),
				OCICompatHosts: viper.GetStringSlice(config.KeyReportOCICompatHosts),
			})
		},
	)

	cmd.Flags().Bool(ociCompatFlagName, false, "push with only widely accepted media types, for registries such as GitLab's (config report.oci.compat)")

	return cmd
}

// readOCICompat returns --oci-compat when it is given, else report.oci.compat
// from GOOZE_REPORT_OCI_COMPAT or gooze.yaml.
func readOCICompat(cmd *cobra.Command) bool {
	if cmd.Flags().Changed(ociCompatFlagName) {
		value, _ := cmd.Flags().GetBool(ociCompatFlagName)

		return value
	}

	return viper.GetBool(config.KeyReportOCICompat)
}
