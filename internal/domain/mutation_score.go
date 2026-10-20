package domain

import (
	m "gooze.dev/pkg/gooze/internal/model"
	pkg "gooze.dev/pkg/gooze/pkg"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// mutationScoreFromReports returns the killed/scored ratio in the 0..1 range
// the reporters expect (they render it as a percentage), see rpt.Score.
func mutationScoreFromReports(reports pkg.FileSpill[m.Report]) (float64, error) {
	var summary rpt.Summary

	err := reports.Range(func(_ uint64, report m.Report) error {
		summary.Merge(summarizeReport(report))
		return nil
	})
	if err != nil {
		return 0.0, err
	}

	return summary.Score(), nil
}

func summarizeReport(report m.Report) rpt.Summary {
	var summary rpt.Summary

	for _, entries := range report.Result {
		for _, entry := range entries {
			summary.Add(entry.Status)
		}
	}

	return summary
}
