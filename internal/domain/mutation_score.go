package domain

import (
	m "gooze.dev/pkg/gooze/internal/model"
	pkg "gooze.dev/pkg/gooze/pkg"
)

func mutationScoreFromReports(reports pkg.FileSpill[m.Report]) (float64, error) {
	killed := 0
	total := 0

	err := reports.Range(func(_ uint64, report m.Report) error {
		k, t := countReport(report)
		killed += k
		total += t

		return nil
	})
	if err != nil {
		return 0.0, err
	}

	return mutationScore(killed, total), nil
}

func mutationScoreFromReportSlice(reports []m.Report) float64 {
	killed := 0
	total := 0

	for _, report := range reports {
		k, t := countReport(report)
		killed += k
		total += t
	}

	return mutationScore(killed, total)
}

func countReport(report m.Report) (killed, total int) {
	for _, entries := range report.Result {
		for _, entry := range entries {
			total++

			if entry.Status == m.Killed {
				killed++
			}
		}
	}

	return killed, total
}

// mutationScore returns the killed/total ratio in the 0..1 range the reporters
// expect (they render it as a percentage). An empty set scores a full 1.0.
func mutationScore(killed, total int) float64 {
	if total == 0 {
		return 1.0
	}

	return float64(killed) / float64(total)
}
