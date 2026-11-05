package domain

import (
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// scopeMetrics is the score of one scope in a metrics report.
type scopeMetrics struct {
	score rpt.Summary
	diff  bool
}

// metrics returns the scope's metrics, then the changed lines' of
// args.DiffScore when a merge reports both scopes.
func (s viewScope) metrics(score rpt.Summary) []scopeMetrics {
	metrics := []scopeMetrics{{score: score, diff: s.diff}}
	if s.args.DiffScore != nil {
		metrics = append(metrics, scopeMetrics{score: *s.args.DiffScore, diff: true})
	}

	return metrics
}

// writeMetrics writes the scores as a GitLab metrics report: one "<name>
// <value>" line per metric, in the OpenMetrics text format, with one # EOF.
// The changed lines' metrics are named gooze_diff_*, since their score
// covers only the changed lines and GitLab compares metrics by name with the
// target branch's. Without mutations there is no score line, only the counts.
func writeMetrics(w io.Writer, scores ...scopeMetrics) error {
	var out strings.Builder

	for _, scope := range scores {
		writeScopeMetrics(&out, scope.score, scope.diff)
	}

	out.WriteString("# EOF\n")

	if _, err := io.WriteString(w, out.String()); err != nil {
		return fmt.Errorf("write metrics: %w", err)
	}

	return nil
}

// writeScopeMetrics writes the metric lines of one scope's score.
func writeScopeMetrics(out *strings.Builder, score rpt.Summary, diff bool) {
	prefix := "gooze_"
	if diff {
		prefix = "gooze_diff_"
	}

	// Equivalent mutants: counts are never negative.
	if score.Total > 0 { //gooze:ignore comparison
		percent := math.Round(score.Score()*10000) / 100
		fmt.Fprintf(out, "%smutation_score_percent %s\n", prefix, strconv.FormatFloat(percent, 'f', -1, 64))
	}

	for _, metric := range []struct {
		name  string
		count int
	}{
		{"total", score.Total},
		{"killed", score.Killed},
		{"survived", score.Survived},
		{"not_covered", score.NotCovered},
		{"timeout", score.Timeout},
		{"skipped", score.Skipped},
		{"error", score.Error},
	} {
		fmt.Fprintf(out, "%smutants_%s %d\n", prefix, metric.name, metric.count)
	}
}
