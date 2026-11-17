package controller

import (
	"fmt"
	"strings"

	"gooze.dev/pkg/gooze/internal/domain"
)

// writeSurvivors lists the summary's survivors, once per line of a file when it
// has them per line. With MaxPerFile, a file lists the survivors of that many
// lines, then how many more lines it has.
func writeSurvivors(out *strings.Builder, summary domain.ReportSummary, section logSection) {
	if len(summary.Survivors) == 0 {
		out.WriteString("No survivors.\n")
		return
	}

	lines := len(summary.SurvivorLines)
	// Equivalent: a count of lines is never negative, so != 0 is > 0.
	folded := lines > 0 //gooze:ignore comparison

	heading := fmt.Sprintf("Survivors (%d):", len(summary.Survivors))
	if folded {
		heading = fmt.Sprintf("Survivors (%d on %d %s):", len(summary.Survivors), lines, lineNoun(lines))
	}

	section.list("survivors", heading, func() {
		for _, file := range domain.ListedLines(summary.Survivors, summary.MaxPerFile) {
			for _, line := range file.Lines {
				writeSurvivorLine(out, line, folded)
			}

			// Equivalent: a count is never negative, so != 0 is > 0.
			if file.Hidden > 0 { //gooze:ignore comparison
				fmt.Fprintf(out, "%s: and %d more %s\n", file.Path, file.Hidden, lineNoun(file.Hidden))
			}
		}
	})
}

// writeSurvivorLine writes the survivors of one line: folded into one entry,
// or one entry each.
func writeSurvivorLine(out *strings.Builder, line domain.SurvivorLine, folded bool) {
	if folded {
		out.WriteString(formatSurvivorLine(line) + "\n")
		return
	}

	for _, survivor := range line.Survivors {
		out.WriteString(formatSurvivor(survivor) + "\n")
	}
}

// lineNoun is "line" for one and "lines" for any other count.
func lineNoun(n int) string {
	// Equivalent: no caller counts zero lines, so <= 1 is == 1.
	if n == 1 { //gooze:ignore comparison
		return "line"
	}

	return "lines"
}
