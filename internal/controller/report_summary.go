package controller

import (
	"bytes"
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/tw"
	"gooze.dev/pkg/gooze/internal/domain"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// lineRemoved stands for the mutated code of a mutation that removes the line.
const lineRemoved = "(line removed)"

// renderReportSummary renders stored reports as plain text: a per-file table,
// one "path:line: status (mutagen): original -> mutated" line per survivor and
// per mutation that did not build, and the mutation score. With SurvivorLines,
// it lists the survivors once per line of a file instead.
func renderReportSummary(summary domain.ReportSummary) string {
	var out strings.Builder

	out.WriteString(renderFileSummaryTable(summary))
	out.WriteString("\n")

	writeSurvivors(&out, summary)

	if len(summary.BuildErrors) > 0 {
		fmt.Fprintf(&out, "\nBuild errors (%d):\n", len(summary.BuildErrors))

		for _, buildError := range summary.BuildErrors {
			out.WriteString(formatSurvivor(buildError))
			out.WriteString("\n")
		}
	}

	fmt.Fprintf(&out, "\nMutation score: %s\n", formatMutationScore(summary.Score))

	if summary.Hint != "" {
		fmt.Fprintf(&out, "\n%s\n", summary.Hint)
	}

	return out.String()
}

// writeSurvivors lists the summary's survivors, once per line of a file when it
// has them per line.
func writeSurvivors(out *strings.Builder, summary domain.ReportSummary) {
	switch lines := len(summary.SurvivorLines); {
	case len(summary.Survivors) == 0:
		out.WriteString("No survivors.\n")
	case lines == 0:
		fmt.Fprintf(out, "Survivors (%d):\n", len(summary.Survivors))

		for _, survivor := range summary.Survivors {
			out.WriteString(formatSurvivor(survivor))
			out.WriteString("\n")
		}
	default:
		noun := "lines"
		// Equivalent: this case has at least one line, so <= 1 is == 1.
		if lines == 1 { //gooze:ignore comparison
			noun = "line"
		}

		fmt.Fprintf(out, "Survivors (%d on %d %s):\n", len(summary.Survivors), lines, noun)

		for _, line := range summary.SurvivorLines {
			out.WriteString(formatSurvivorLine(line))
			out.WriteString("\n")
		}
	}
}

func renderFileSummaryTable(summary domain.ReportSummary) string {
	var buf bytes.Buffer

	table := tablewriter.NewTable(&buf,
		tablewriter.WithRendition(tw.Rendition{
			Borders: tw.Border{Left: tw.Off, Right: tw.Off, Top: tw.Off, Bottom: tw.Off},
		}),
		tablewriter.WithAlignment(tw.Alignment{tw.AlignLeft, tw.AlignRight, tw.AlignRight, tw.AlignRight, tw.AlignRight, tw.AlignRight, tw.AlignRight, tw.AlignRight}),
		// Auto-formatting would space out the score's decimal point.
		tablewriter.WithFooterAutoFormat(tw.Off),
	)
	table.Header("Path", "Mutations", "Killed", "Survived", "Not covered", "Timeout", "Error", "Score")

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
		fmt.Sprintf("%d", counts.Timeout),
		fmt.Sprintf("%d", counts.Error),
		fmt.Sprintf("%.2f%%", counts.Score()*100),
	}
}

// survivorLocation returns "path:line", or the path when the report has no line.
func survivorLocation(survivor domain.Survivor) string {
	// Equivalent mutants: Line is never negative.
	if survivor.Line > 0 { //gooze:ignore comparison
		return fmt.Sprintf("%s:%d", survivor.Path, survivor.Line)
	}

	return survivor.Path
}

// formatSurvivor renders a survivor as "path:line: status (mutagen): original
// -> mutated", leaving out the location details the report does not have.
func formatSurvivor(survivor domain.Survivor) string {
	text := fmt.Sprintf("%s: %s (%s)", survivorLocation(survivor), survivor.Status, survivor.Mutagen)

	switch {
	case survivor.Original == "":
		return text
	case survivor.Mutated == "":
		return fmt.Sprintf("%s: %s -> %s", text, survivor.Original, lineRemoved)
	default:
		return fmt.Sprintf("%s: %s -> %s", text, survivor.Original, survivor.Mutated)
	}
}

// formatSurvivorLine renders the survivors of one line as "path:line: 5 survived
// (comparison ×4, numbers): original -> >= 0, == 0, <= 0, < 0, > 1". A lone
// survivor reads as it does in the flat list.
func formatSurvivorLine(line domain.SurvivorLine) string {
	first := line.Survivors[0]

	// Equivalent: a line has at least one survivor, so <= 1 is == 1.
	if len(line.Survivors) == 1 { //gooze:ignore comparison
		return formatSurvivor(first)
	}

	statuses := tally(line.Survivors, func(s domain.Survivor) string { return s.Status.String() }, "%[2]d %[1]s", "%[2]d %[1]s")
	mutagens := tally(line.Survivors, func(s domain.Survivor) string { return s.Mutagen }, "%[1]s", "%[1]s ×%[2]d")
	text := fmt.Sprintf("%s: %s (%s)", survivorLocation(first), statuses, mutagens)

	if first.Original == "" {
		return text
	}

	mutated := make([]string, 0, len(line.Survivors))
	for _, survivor := range line.Survivors {
		mutated = append(mutated, survivor.Mutated)
	}

	return fmt.Sprintf("%s: %s -> %s", text, first.Original, strings.Join(foldChanges(first.Original, mutated), ", "))
}

// tally lists the distinct keys of survivors in the order they first appear,
// each written with its count through the format for a single or several
// survivors.
func tally(survivors []domain.Survivor, key func(domain.Survivor) string, single, several string) string {
	counts := map[string]int{}

	var keys []string

	for _, survivor := range survivors {
		name := key(survivor)
		// Equivalent: a count is never negative, so <= 0 is == 0.
		if counts[name] == 0 { //gooze:ignore comparison
			keys = append(keys, name)
		}

		counts[name]++
	}

	parts := make([]string, 0, len(keys))

	for _, name := range keys {
		format := several
		// Equivalent: a listed key has at least one survivor, so <= 1 is == 1.
		if counts[name] == 1 { //gooze:ignore comparison
			format = single
		}

		parts = append(parts, fmt.Sprintf(format, name, counts[name]))
	}

	return strings.Join(parts, ", ")
}

// foldChanges returns what differs in each mutated line, dropping the
// space-separated tokens at either end that the original and every mutated line
// share. Each keeps at least one token, and a removed line ("") reads as such.
func foldChanges(original string, mutated []string) []string {
	lines := [][]string{strings.Split(original, " ")}

	for _, text := range mutated {
		if text != "" {
			lines = append(lines, strings.Split(text, " "))
		}
	}

	limit := len(slices.MinFunc(lines, func(a, b []string) int { return cmp.Compare(len(a), len(b)) })) - 1
	prefix := commonTokens(lines, limit, func(_ []string, i int) int { return i })
	suffix := commonTokens(lines, limit-prefix, func(line []string, i int) int { return len(line) - 1 - i })

	folded := make([]string, 0, len(mutated))

	for _, text := range mutated {
		if text == "" {
			folded = append(folded, lineRemoved)
			continue
		}

		tokens := strings.Split(text, " ")
		folded = append(folded, strings.Join(tokens[prefix:len(tokens)-suffix], " "))
	}

	return folded
}

// commonTokens counts, up to limit, the leading steps at which every line has
// the same token; index says which token a step reads in a line.
func commonTokens(lines [][]string, limit int, index func(line []string, step int) int) int {
	for step := range limit {
		token := lines[0][index(lines[0], step)]
		if slices.ContainsFunc(lines, func(line []string) bool { return line[index(line, step)] != token }) {
			return step
		}
	}

	return limit
}
