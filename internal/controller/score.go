package controller

import (
	"fmt"
	"strings"

	"gooze.dev/pkg/gooze/internal/domain"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// formatMutationScore renders a score with the counts behind it, e.g.
// "87.50% (14 killed, 2 survived; 16 cached, 0 tested)". Statuses other than
// killed and survived appear only when present; the cached/tested split only
// for a run.
// A narrowed run's score names its selection last, then whether an
// interruption or a fail-fast policy left mutants untested.
func formatMutationScore(score domain.MutationScore) string {
	breakdown := scoreBreakdown(score)
	if score.Selection != "" {
		breakdown += "; " + score.Selection
	}

	switch {
	case score.Interrupted:
		breakdown += fmt.Sprintf("; interrupted after %d of %d", score.Tested, score.Planned)
	case score.FailFast == domain.FailFastFile:
		breakdown += fmt.Sprintf("; fail-fast %s skipped %d of %d", score.FailFast, score.Planned-score.Tested, score.Planned)
	case score.FailFast != "":
		breakdown += fmt.Sprintf("; stopped early by fail-fast %s after %d of %d", score.FailFast, score.Tested, score.Planned)
	}

	return fmt.Sprintf("%.2f%% (%s)", score.Score()*100, breakdown)
}

// runReportsHint tells where a narrowed run wrote its reports and how to add
// them to the main reports, and that a shard left its thresholds to merge.
func runReportsHint(score domain.MutationScore) string {
	reports := ""
	if score.ReportsDir != "" {
		reports = fmt.Sprintf("Reports of this run are in %s; add them to the main reports with gooze report merge.", score.ReportsDir)
	}

	thresholds := ""
	if score.ThresholdsSkipped {
		thresholds = "Thresholds are not checked on a shard; gooze report merge checks them on all the results."
	}

	return joinLines(reports, thresholds)
}

func scoreBreakdown(score domain.MutationScore) string {
	// Equivalent mutants: counts and lengths are never negative.
	if score.Total == 0 { //gooze:ignore comparison
		return "no mutations"
	}

	parts := []string{
		fmt.Sprintf("%d %s", score.Killed, rpt.Killed),
		fmt.Sprintf("%d %s", score.Survived, rpt.Survived),
	}

	optional := []struct {
		count  int
		status rpt.TestStatus
	}{
		{score.NotCovered, rpt.NotCovered},
		{score.Timeout, rpt.Timeout},
		{score.Skipped, rpt.Skipped},
		{score.Error, rpt.Error},
	}

	for _, entry := range optional {
		// Equivalent mutants: counts and lengths are never negative.
		if entry.count > 0 { //gooze:ignore comparison
			parts = append(parts, fmt.Sprintf("%d %s", entry.count, entry.status))
		}
	}

	text := strings.Join(parts, ", ")
	// Equivalent mutants: counts and lengths are never negative.
	if score.Cached > 0 || score.Tested > 0 { //gooze:ignore comparison
		text += fmt.Sprintf("; %d cached, %d tested", score.Cached, score.Tested)
	}

	return text
}

// cachedEstimateHint explains the cached files of an estimation, or returns ""
// when none are cached.
func cachedEstimateHint(estimation domain.Estimation) string {
	// Equivalent mutants: counts and lengths are never negative.
	if estimation.Cached == 0 { //gooze:ignore comparison
		return ""
	}

	return fmt.Sprintf(
		"%d of %d mutations are in cached files and will not be re-tested; use --no-cache to re-test everything.",
		estimation.Cached, estimation.Total,
	)
}

// shortCommitLen is how many characters of a commit hash diff mode shows.
const shortCommitLen = 7

// diffEstimateLine describes a diff-mode estimate, or returns "" outside diff
// mode.
func diffEstimateLine(estimation domain.Estimation) string {
	diff := estimation.Diff
	if diff == nil {
		return ""
	}

	base := diff.Base
	// Equivalent: a base of exactly shortCommitLen is unchanged by the cut.
	if len(base) > shortCommitLen { //gooze:ignore comparison
		base = base[:shortCommitLen]
	}

	// Equivalent mutant: file counts are never negative.
	if diff.Files == 0 { //gooze:ignore comparison
		return fmt.Sprintf("Diff mode: no changed files since %s (merge base %s).", diff.Ref, base)
	}

	files := "files"
	// Equivalent: zero files returned above, and counts are never negative.
	if diff.Files == 1 { //gooze:ignore comparison
		files = "file"
	}

	return fmt.Sprintf("Diff mode: %d %s changed since %s (merge base %s); counts cover changed lines only.",
		diff.Files, files, diff.Ref, base)
}

// selectionLine names the --run pattern and mutagen subset an estimate is
// limited to, or returns "" when there are none.
func selectionLine(estimation domain.Estimation) string {
	var parts []string

	if estimation.Run != "" {
		parts = append(parts, "functions matching "+estimation.Run)
	}

	if len(estimation.Mutagens) > 0 {
		parts = append(parts, "mutagens "+strings.Join(estimation.Mutagens, ", "))
	}

	if len(parts) == 0 {
		return ""
	}

	return "Only " + strings.Join(parts, ", ") + "."
}
