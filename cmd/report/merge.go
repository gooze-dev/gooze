package report

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
)

// scopeTotal is the whole project's scope, the default of --scope.
const scopeTotal = "total"

// parseScope reads --scope: total, or both scopes. --write-diff needs both.
func parseScope(value string, writesDiff bool) (bool, error) {
	var scopes bool

	switch value {
	case scopeTotal:
	case "diff,total", "total,diff":
		scopes = true
	case "diff":
		return false, domain.WithExitCode(m.ExitUsage,
			fmt.Errorf("invalid --scope %q: must be total or diff,total; for the changed lines alone, merge -o <reports>/run", value))
	default:
		return false, domain.WithExitCode(m.ExitUsage, fmt.Errorf("invalid --scope %q: must be total or diff,total", value))
	}

	if writesDiff && !scopes {
		return false, domain.WithExitCode(m.ExitUsage, errors.New("--write-diff writes the changed lines' scope; add --scope diff,total"))
	}

	return scopes, nil
}

const mergeLongDescription = `Merge reports from shard_* subdirectories into a single reports directory.

Each shard dir holds the results its run tested and copies of the stored
(cached) results of the unchanged sources it owns, so the merge gives the same
score as an unsharded run, also when only the shard dirs reach the merge job.

Then it shows the merged reports as report view does: the per-file table,
the survivors and the score, or less with --summary and --top, in --format.
Repeat --write <format>=<path> to write more reports from the same load, such
as --write gitlab=gl-code-quality.json --write metrics=metrics.txt.

--scope diff,total reports both scopes of a merge request in one command: it
merges the --diff shards in <reports>/run and shows the changed lines
(--write-diff writes their files), then folds them into the baseline in
<reports>, keeps the merged run dir for report view -o <reports>/run, and
shows the whole project (--write). The metrics of the whole project then add
gooze_diff_* for the changed lines, with one # EOF. The gate defaults to the
changed lines; --format applies to the whole project.

Below a --threshold-* minimum, or with more surviving mutants than
--threshold-max-survivors, merge exits with code 3. For the shards of a gooze
run --diff (merge -o .gooze-reports/run), the --threshold-diff-* minimums
apply instead, as they do for report view. --gate (report.gate) picks the
scope whose thresholds decide the exit code: auto (the reports' own scope),
diff, total or none.`

func newMergeCmd(deps Deps) *cobra.Command {
	var (
		flags                      viewFlags
		scope                      string
		diffWrites                 []string
		thresholds, diffThresholds ThresholdFlags
	)

	cmd := &cobra.Command{
		Use:   "merge",
		Short: "Merge sharded reports into a single directory",
		Long:  mergeLongDescription,
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			minimums, diffMinimums, err := ReadGates(cmd, thresholds, diffThresholds)
			if err != nil {
				return err
			}

			scopes, err := parseScope(scope, len(diffWrites) > 0)
			if err != nil {
				return err
			}

			view, closeWrites, err := flags.viewArgs(cmd)
			if err != nil {
				return err
			}

			writes, closeDiffWrites, err := openWrites(diffWrites)
			if err != nil {
				return errors.Join(err, closeWrites())
			}

			err = deps.Workflow.Merge(cmd.Context(), domain.MergeArgs{
				Reports: m.Path(viper.GetString(deps.OutputKey)), View: view, Scopes: scopes, DiffWrites: writes,
				Thresholds: minimums, DiffThresholds: diffMinimums,
			})

			return errors.Join(err, closeWrites(), closeDiffWrites())
		},
	}

	flags.register(cmd)
	cmd.Flags().StringVar(&scope, "scope", scopeTotal, "the scopes to report: total, or diff,total to merge the --diff shards in <reports>/run, show the changed lines, then fold them in and show the whole project")
	cmd.Flags().StringArrayVar(&diffWrites, "write-diff", nil, "with --scope diff,total, also write the changed lines' report as <format>=<path>, repeatable")
	addFormatFlag(cmd)
	thresholds = AddThresholdFlags(cmd, "threshold", "threshold")
	diffThresholds = AddThresholdFlags(cmd, "threshold.diff", "threshold-diff")

	return cmd
}
