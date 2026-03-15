package v2

import (
	"log/slog"

	"github.com/spf13/cobra"
	"gooze.dev/pkg/gooze/internal/adapter"
	"gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
)

var workflow domain.WorkflowPipeline
var events m.Events

func Root(cmd *cobra.Command, args []string) error {
	slog.Info("No specified command. Use --help to see available commands.")
	return cmd.Help()
}

func Initialize(cmd *cobra.Command, args []string) {
	events = domain.NewEvents()
	fileAdapter := adapter.NewFilesAdapter(events)
	goFileAdapter := adapter.NewLocalGoFileAdapter()
	mutagen := domain.NewMutagen(goFileAdapter, fileAdapter, events)
	workflow = domain.NewWorkflowPipeline(events, fileAdapter, mutagen)
}
