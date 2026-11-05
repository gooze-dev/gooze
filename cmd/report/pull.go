package report

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
)

func newPullCmd(deps Deps) *cobra.Command {
	var cmd *cobra.Command

	cmd = newTransferCmd(
		"pull <reference>",
		"Pull reports from an OCI registry",
		"Pull a reports artifact from an OCI registry and replace the stored reports in the reports\n"+
			"directory with it, so stale local reports don't change the score. shard_* dirs and the run\n"+
			"dir of a --diff or --run run are kept: gooze report merge folds them in. It prints the\n"+
			"artifact's digest, how many report files it holds and their mutation score.",
		func(ctx context.Context, ref string, opts registryFlags) error {
			result, err := deps.Publisher.Pull(ctx, domain.PullArgs{
				Reports:   m.Path(viper.GetString(deps.OutputKey)),
				Ref:       ref,
				PlainHTTP: opts.plainHTTP,
				Insecure:  opts.insecure,
			})
			if err != nil {
				return err
			}

			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Pulled %s (%s): %d report files, mutation score %s\n",
				ref, result.Digest, result.Reports, pulledScore(result.Score))

			return err
		},
	)

	return cmd
}

// pulledScore is a pulled baseline's score, such as "66.67% (2 killed, 1
// survived)".
func pulledScore(score domain.MutationScore) string {
	// Equivalent mutant: counts are never negative.
	if score.Total == 0 { //gooze:ignore comparison
		return "n/a (no mutations)"
	}

	return fmt.Sprintf("%.2f%% (%d killed, %d survived)", score.Score()*100, score.Killed, score.Survived)
}
