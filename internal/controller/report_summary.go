package controller

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/tw"
	"gooze.dev/pkg/gooze/internal/domain"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// renderReportSummary renders stored reports as plain text: a per-file table,
// one "path:line: status (mutagen): original -> mutated" line per survivor, and
// the mutation score.
func renderReportSummary(summary domain.ReportSummary) string {
	var out strings.Builder

	out.WriteString(renderFileSummaryTable(summary))
	out.WriteString("\n")

	if len(summary.Survivors) == 0 {
		out.WriteString("No survivors.\n")
	} else {
		fmt.Fprintf(&out, "Survivors (%d):\n", len(summary.Survivors))

		for _, survivor := range summary.Survivors {
			out.WriteString(formatSurvivor(survivor))
			out.WriteString("\n")
		}
	}

	fmt.Fprintf(&out, "\nMutation score: %s\n", formatMutationScore(summary.Score))

	return out.String()
}

func renderFileSummaryTable(summary domain.ReportSummary) string {
	var buf bytes.Buffer

	table := tablewriter.NewTable(&buf,
		tablewriter.WithRendition(tw.Rendition{
			Borders: tw.Border{Left: tw.Off, Right: tw.Off, Top: tw.Off, Bottom: tw.Off},
		}),
		tablewriter.WithAlignment(tw.Alignment{tw.AlignLeft, tw.AlignRight, tw.AlignRight, tw.AlignRight, tw.AlignRight, tw.AlignRight}),
		// Auto-formatting would space out the score's decimal point.
		tablewriter.WithFooterAutoFormat(tw.Off),
	)
	table.Header("Path", "Mutations", "Killed", "Survived", "Not covered", "Score")

	for _, file := range summary.Files {
		if err := table.Append(summaryRow(file.Path, file.Summary)); err != nil {
			return fmt.Sprintf("failed to render report summary: %v\n", err)
		}
	}

	table.Footer(summaryRow(fmt.Sprintf("TOTAL FILES %d", len(summary.Files)), summary.Score.Summary))

	if err := table.Render(); err != nil {
		return fmt.Sprintf("failed to render report summary: %v\n", err)
	}

	return buf.String()
}

func summaryRow(label string, counts rpt.Summary) []string {
	return []string{
		label,
		fmt.Sprintf("%d", counts.Total),
		fmt.Sprintf("%d", counts.Killed),
		fmt.Sprintf("%d", counts.Survived),
		fmt.Sprintf("%d", counts.NotCovered),
		fmt.Sprintf("%.2f%%", counts.Score()*100),
	}
}

// formatSurvivor renders a survivor as "path:line: status (mutagen): original
// -> mutated", leaving out the location details the report does not have.
func formatSurvivor(survivor domain.Survivor) string {
	location := survivor.Path
	// Equivalent mutants: Line is never negative.
	if survivor.Line > 0 { //gooze:ignore comparison
		location = fmt.Sprintf("%s:%d", survivor.Path, survivor.Line)
	}

	text := fmt.Sprintf("%s: %s (%s)", location, survivor.Status, survivor.Mutagen)

	switch {
	case survivor.Original == "":
		return text
	case survivor.Mutated == "":
		return fmt.Sprintf("%s: %s -> (line removed)", text, survivor.Original)
	default:
		return fmt.Sprintf("%s: %s -> %s", text, survivor.Original, survivor.Mutated)
	}
}
