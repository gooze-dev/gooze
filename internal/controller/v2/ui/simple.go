package ui

import (
	"context"
	"fmt"
	"io"

	m "gooze.dev/pkg/gooze/internal/model"
)

type SimpleUI struct {
	Writer io.Writer
}



func NewSimpleUI(w io.Writer) UI {
	return &SimpleUI{
		Writer: w,
	}
}

func (ui *SimpleUI) ShowEstimateProgress(ctx context.Context, events m.Events) {
	for {
		select {
		case event, ok := <-events.Chan():
			if !ok {
				return
			}
			switch event.Kind {
			case "start-scan":
				fmt.Fprintln(ui.Writer, "Scanning paths...")
			case "scan":
				fmt.Fprintf(ui.Writer, "  Scanning %s\n", event.Path)
			case "finish-scan":
				fmt.Fprintln(ui.Writer, "Finished scanning paths.")
			case "error":
				fmt.Fprintf(ui.Writer, "Error: %v\n", event.Err)
				return
			case "start-generate":
				fmt.Fprintln(ui.Writer, "Generating mutations...")
			case "generating-mutations":
				fmt.Fprintf(ui.Writer, "  Generating mutations for %s\n", event.Source.Origin.ShortPath)
			case "finish-generate":
				fmt.Fprintln(ui.Writer, "Finished generating mutations.")

			case "start-estimate":
				fmt.Fprintln(ui.Writer, "Estimating mutations...")
			case "estimate":
				fmt.Fprintf(ui.Writer, "  Estimating [%s] %s\n", event.Mutation.Type.Name, event.Mutation.Source.Origin.ShortPath)
			case "finish-estimate":
				fmt.Fprintln(ui.Writer, "Finished estimating.")
			case "pair-found":
				fmt.Fprintf(ui.Writer, "  Pair found: %s\n", event.Source.Origin.ShortPath)
			case "show-estimation-result":
				fmt.Fprintln(ui.Writer, "Estimation results:")
				for _, data := range event.Result {
					fmt.Fprintf(ui.Writer, "  %s: %d mutations (source: %s)\n", data.Source.Origin.ShortPath, data.Count, data.Source.Origin.FullPath)
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

// ShowEstimateResults implements UI.
func (ui *SimpleUI) ShowEstimateResults(ctx context.Context, events m.Events) {
	panic("unimplemented")
}

// ShowTestExecutionProgress implements UI.
func (ui *SimpleUI) ShowTestExecutionProgress(ctx context.Context, events m.Events) {
	panic("unimplemented")
}

// ShowTestExecutionResults implements UI.
func (ui *SimpleUI) ShowTestExecutionResults(ctx context.Context, events m.Events) {
	panic("unimplemented")
}
