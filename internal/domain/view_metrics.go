package domain

import (
	"context"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	m "gooze.dev/pkg/gooze/internal/model"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// writeMetrics writes score as a GitLab metrics report: one "<name> <value>"
// line per metric, in the OpenMetrics text format. A --diff run's metrics are
// named gooze_diff_*, since its score covers only the changed lines and
// GitLab compares metrics by name with the target branch's. Without
// mutations there is no score line, only the counts.
func writeMetrics(w io.Writer, score rpt.Summary, diff bool) error {
	prefix := "gooze_"
	if diff {
		prefix = "gooze_diff_"
	}

	var out strings.Builder

	// Equivalent mutants: counts are never negative.
	if score.Total > 0 { //gooze:ignore comparison
		percent := math.Round(score.Score()*10000) / 100
		fmt.Fprintf(&out, "%smutation_score_percent %s\n", prefix, strconv.FormatFloat(percent, 'f', -1, 64))
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
		fmt.Fprintf(&out, "%smutants_%s %d\n", prefix, metric.name, metric.count)
	}

	out.WriteString("# EOF\n")

	if _, err := io.WriteString(w, out.String()); err != nil {
		return fmt.Errorf("write metrics: %w", err)
	}

	return nil
}

// viewMetrics writes the metrics of summary, named for a --diff run when the
// viewed reports are one's.
func (w *workflow) viewMetrics(ctx context.Context, reports m.Path, out io.Writer, summary ReportSummary) error {
	selection, err := w.reports.LoadSelection(ctx, reports)
	if err != nil {
		return fmt.Errorf("load the run's selection: %w", err)
	}

	return writeMetrics(out, summary.Score.Summary, selection.Diff != "")
}
