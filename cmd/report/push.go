package report

import (
	"context"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
	"gooze.dev/pkg/gooze/pkg/config"
)

const (
	ociCompatFlagName = "oci-compat"
	// ociCompatEnv sets report.oci.compat (viper reads it as GOOZE_ + the key).
	ociCompatEnv = "GOOZE_REPORT_OCI_COMPAT"
)

func newPushCmd(deps Deps) *cobra.Command {
	var cmd *cobra.Command

	cmd = newTransferCmd(
		"push <reference>",
		"Push reports to an OCI registry",
		"Package the reports directory and push it to an OCI registry as an artifact.\n\n"+
			"Registries that accept only allowlisted media types, such as GitLab's, need compat media types:\n"+
			"--oci-compat (config report.oci.compat) turns them on. Unless it is set, they are on by\n"+
			"themselves for registry.gitlab.com and the hosts in report.oci.compat_hosts;\n"+
			"--oci-compat=false turns them off there too.",
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
// when GOOZE_REPORT_OCI_COMPAT or gooze.yaml sets it, else nil: compat media
// types are then used for the hosts that need them.
func readOCICompat(cmd *cobra.Command) *bool {
	if cmd.Flags().Changed(ociCompatFlagName) {
		value, _ := cmd.Flags().GetBool(ociCompatFlagName)

		return &value
	}

	if os.Getenv(ociCompatEnv) == "" && !viper.InConfig(config.KeyReportOCICompat) {
		return nil
	}

	value := viper.GetBool(config.KeyReportOCICompat)

	return &value
}
