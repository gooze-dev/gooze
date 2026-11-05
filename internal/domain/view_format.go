package domain

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"

	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// ViewFormat is how report view writes the mutations that lower the score.
type ViewFormat string

const (
	// FormatText lists survivors in the text summary.
	FormatText ViewFormat = "text"
	// FormatGitHub writes a GitHub workflow annotation per line with survivors,
	// then the text summary.
	FormatGitHub ViewFormat = "github"
	// FormatSARIF writes a SARIF 2.1.0 log instead of the text summary.
	FormatSARIF ViewFormat = "sarif"
	// FormatCodeClimate writes a Code Climate JSON report, which GitLab reads
	// as a Code Quality report, instead of the text summary.
	FormatCodeClimate ViewFormat = "codeclimate"
	// FormatGitLab is FormatCodeClimate under the name of the platform that
	// reads it.
	FormatGitLab ViewFormat = "gitlab"
	// FormatMetrics writes the score and counts as a GitLab metrics report
	// instead of the text summary.
	FormatMetrics ViewFormat = "metrics"
)

// displayView writes summary in args.Format.
func (w *workflow) displayView(ctx context.Context, scope viewScope, summary ReportSummary) error {
	args := scope.args
	if args.Format == FormatGitHub {
		for _, annotation := range gitHubAnnotations(summary.Survivors, args.PathPrefix) {
			if _, err := fmt.Fprintln(args.Output, annotation); err != nil {
				return fmt.Errorf("write annotations: %w", err)
			}
		}
	}

	if written, err := writeMachineFormat(scope, args.Format, args.Output, summary); written || err != nil {
		return err
	}

	w.progress.DisplayReportSummary(ctx, summary)

	return nil
}

// writeView writes summary to a report file in its format; text as the
// whole summary.
func (w *workflow) writeView(ctx context.Context, scope viewScope, write ReportWrite, summary ReportSummary) error {
	if written, err := writeMachineFormat(scope, write.Format, write.Output, summary); written || err != nil {
		return err
	}

	if err := w.progress.WriteReportSummary(ctx, write.Output, summary); err != nil {
		return fmt.Errorf("write the text report: %w", err)
	}

	return nil
}

// writeMachineFormat writes summary to out in format when it is one of the
// formats for tools (SARIF, Code Climate, metrics), and reports whether it
// was.
func writeMachineFormat(scope viewScope, format ViewFormat, out io.Writer, summary ReportSummary) (bool, error) {
	args := scope.args

	switch format {
	case FormatSARIF:
		return true, writeSARIF(out, summary.Survivors, args.PathPrefix, args.ToolVersion)
	case FormatCodeClimate, FormatGitLab:
		return true, writeCodeClimate(out, summary.Survivors, args.PathPrefix)
	case FormatMetrics:
		return true, writeMetrics(out, scope.metrics(summary.Score.Summary)...)
	default:
		return false, nil
	}
}

// lineKey is a line of a file.
type lineKey struct {
	Path string
	Line int
}

// SurvivorLine is the survivors on one line of a file, in summary order; Path
// and Line come from the embedded lineKey.
type SurvivorLine struct {
	lineKey

	Survivors []Survivor
}

// groupByLine groups survivors sorted by path and line into one entry per line.
func groupByLine(survivors []Survivor) []SurvivorLine {
	var lines []SurvivorLine

	for _, survivor := range survivors {
		key := lineKey{Path: survivor.Path, Line: survivor.Line}
		if n := len(lines); n > 0 && lines[n-1].lineKey == key {
			lines[n-1].Survivors = append(lines[n-1].Survivors, survivor)
			continue
		}

		lines = append(lines, SurvivorLine{lineKey: key, Survivors: []Survivor{survivor}})
	}

	return lines
}

// statusText names how each scored status lets a mutant through: the title of
// a single mutant, the adjective for several, and the start of its message.
var statusText = map[rpt.TestStatus]struct{ title, adjective, message string }{
	rpt.Survived:   {"Surviving mutant", "surviving", "Tests still pass with "},
	rpt.NotCovered: {"Uncovered mutant", "uncovered", "No test covers "},
	rpt.Timeout:    {"Timed-out mutant", "timed-out", "Tests time out with "},
	rpt.Skipped:    {"Skipped mutant", "skipped", "Not tested: "},
}

// survivorMessage describes one mutant, such as "Tests still pass with `a` changed to `b`".
func survivorMessage(survivor Survivor) string {
	change := "this " + survivor.Mutagen + " mutation"

	switch {
	case survivor.Original == "":
	case survivor.Mutated == "":
		change = "`" + survivor.Original + "` removed"
	default:
		change = "`" + survivor.Original + "` changed to `" + survivor.Mutated + "`"
	}

	return statusText[survivor.Status].message + change
}

// lineTitle titles the mutants of one line: "Surviving mutant (comparison)",
// "2 surviving mutants (comparison)", or "3 mutants (arithmetic, comparison)"
// when their statuses differ.
func lineTitle(line SurvivorLine) string {
	var mutagens []string

	for _, survivor := range line.Survivors {
		if !slices.Contains(mutagens, survivor.Mutagen) {
			mutagens = append(mutagens, survivor.Mutagen)
		}
	}

	list := strings.Join(mutagens, ", ")
	status := line.Survivors[0].Status

	// Equivalent: a line has at least one survivor, so <= 1 is == 1.
	if len(line.Survivors) == 1 { //gooze:ignore comparison
		return statusText[status].title + " (" + list + ")"
	}

	sameStatus := !slices.ContainsFunc(line.Survivors, func(s Survivor) bool { return s.Status != status })
	if sameStatus {
		return fmt.Sprintf("%d %s mutants (%s)", len(line.Survivors), statusText[status].adjective, list)
	}

	return fmt.Sprintf("%d mutants (%s)", len(line.Survivors), list)
}

// lineMessage lists the mutants of one line, one per line of text.
func lineMessage(line SurvivorLine) string {
	messages := make([]string, 0, len(line.Survivors))
	for _, survivor := range line.Survivors {
		messages = append(messages, survivorMessage(survivor))
	}

	return strings.Join(messages, "\n")
}

// lineSurvived reports whether a mutant on the line survived its tests, the
// most severe outcome.
func lineSurvived(line SurvivorLine) bool {
	// Equivalent: killed mutants aren't survivors, so <= Survived is == Survived.
	return slices.ContainsFunc(line.Survivors, func(s Survivor) bool { return s.Status == rpt.Survived }) //gooze:ignore comparison
}

// gitHubAnnotations returns one workflow command per line with survivors: a
// warning when a mutant on it survived, a notice when they were only not
// covered, timed out or skipped.
func gitHubAnnotations(survivors []Survivor, prefix string) []string {
	lines := groupByLine(survivors)
	annotations := make([]string, 0, len(lines))

	for _, line := range lines {
		annotations = append(annotations, gitHubAnnotation(line, prefix))
	}

	return annotations
}

func gitHubAnnotation(line SurvivorLine, prefix string) string {
	command := "notice"
	if lineSurvived(line) {
		command = "warning"
	}

	properties := "file=" + escapeAnnotationProperty(prefix+line.Path)
	if line.Line > 0 {
		properties += fmt.Sprintf(",line=%d", line.Line)
	}

	properties += ",title=" + escapeAnnotationProperty(lineTitle(line))

	return "::" + command + " " + properties + "::" + escapeAnnotationData(lineMessage(line))
}

// escapeAnnotationData escapes a workflow command's message as GitHub
// requires.
func escapeAnnotationData(s string) string {
	return strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A").Replace(s)
}

// escapeAnnotationProperty escapes a workflow command property, which also
// can't hold ":" or ",".
func escapeAnnotationProperty(s string) string {
	return strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A", ":", "%3A", ",", "%2C").Replace(s)
}
