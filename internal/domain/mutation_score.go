package domain

import (
	"maps"
	"slices"

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
	// Selection describes a narrowed run's selection (see Selection.Describe);
	// its score then covers only that selection.
	Selection string
	// ReportsDir is where a narrowed run wrote its reports.
	ReportsDir m.Path
	// Files counts the results of each source file, for the thresholds on
	// file scores.
	Files []FileSummary
	// ThresholdsSkipped is set on a shard's score when thresholds are set:
	// a shard holds only part of the results, so report merge checks them.
	ThresholdsSkipped bool
	// Interrupted is set when the run was stopped before it tested every
	// mutation; Planned is then how many it was to test.
	Interrupted bool
	Planned     int
	// FailFast is the policy that left mutants untested, if any.
	FailFast FailFast
}

// scoreReports counts the results of the reports whose source include
// accepts, or of every report when include is nil, in total and per file
// (sorted by path).
func scoreReports(reports pkg.FileSpill[m.Report], include func(rpt.Source) bool) (MutationScore, error) {
	files := map[string]*FileSummary{}

	var score MutationScore

	err := reports.Range(func(_ uint64, report m.Report) error {
		if include != nil && !include(report.Source) {
			return nil
		}

		key := report.Source.Key()

		file, ok := files[key]
		if !ok {
			file = &FileSummary{Path: key}
			files[key] = file
		}

		counts := summarizeReport(report)
		file.Merge(counts)
		score.Merge(counts)

		return nil
	})
	if err != nil {
		return MutationScore{}, err
	}

	for _, key := range slices.Sorted(maps.Keys(files)) {
		score.Files = append(score.Files, *files[key])
	}

	return score, nil
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
