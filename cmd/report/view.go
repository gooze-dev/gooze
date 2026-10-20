package report

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
)

const viewLongDescription = `View previously generated mutation reports from the reports directory (-o).

By default it prints one row per source file (mutations, killed, survived,
not covered, score), then every mutation that counts against the score as
"path:line: status (mutagen): original -> mutated", then the mutation score.

Paths limit the output to matching source files and read like ` + "`gooze run`" + `
arguments, relative to the module root:
  - ./...           every file (the default)
  - ./pkg/...       files under pkg, recursively
  - ./pkg           files directly in pkg
  - ./pkg/file.go   a single file

Use --all to list every mutation with its status and diff instead.`

func newViewCmd(deps Deps) *cobra.Command {
	var all bool

	cmd := &cobra.Command{
		Use:   "view [paths...]",
		Short: "Summarize previously generated mutation reports",
		Long:  viewLongDescription,
		Example: `  gooze report view                      # per-file scores and survivors
  gooze report view ./internal/domain/... # only files under internal/domain
  gooze report view --all ./cmd/run.go    # every mutation of one file`,
		RunE: func(cmd *cobra.Command, args []string) error {
			paths := make([]m.Path, 0, len(args))
			for _, arg := range args {
				paths = append(paths, m.Path(arg))
			}

			return deps.Workflow.View(cmd.Context(), domain.ViewArgs{
				Reports: m.Path(viper.GetString(deps.OutputKey)),
				Paths:   paths,
				All:     all,
			})
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "list every mutation with its status and diff instead of a per-file summary")

	return cmd
}
