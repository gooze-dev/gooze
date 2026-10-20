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
func formatMutationScore(score domain.MutationScore) string {
	return fmt.Sprintf("%.2f%% (%s)", score.Score()*100, scoreBreakdown(score))
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
