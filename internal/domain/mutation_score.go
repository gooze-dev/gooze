package domain

import (
	m "gooze.dev/pkg/gooze/internal/model"
	pkg "gooze.dev/pkg/gooze/pkg"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// MutationScore is a mutation score with the counts it is computed from. For a
// run, Cached and Tested split Summary.Total into results reused from stored
// reports and results of mutations the run tested; both are zero for the score
// of a reports directory (view, merge).
type MutationScore struct {
	rpt.Summary
	Cached int
	Tested int
}

// summarizeReports counts the results of the reports whose source include
// accepts, or of every report when include is nil.
func summarizeReports(reports pkg.FileSpill[m.Report], include func(rpt.Source) bool) (rpt.Summary, error) {
	var summary rpt.Summary

	err := reports.Range(func(_ uint64, report m.Report) error {
		if include == nil || include(report.Source) {
			summary.Merge(summarizeReport(report))
		}

		return nil
	})
	if err != nil {
		return rpt.Summary{}, err
	}

	return summary, nil
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
