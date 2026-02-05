package domain

import (
	m "gooze.dev/pkg/gooze/internal/model"
	pkg "gooze.dev/pkg/gooze/pkg"
)

func mutationScoreFromReports(reports pkg.FileSpill[m.Report]) (float64, error) {
	killed := 0
	total := 0

	err := reports.Range(func(_ uint64, report m.Report) error {
		for _, entries := range report.Result {
			for _, entry := range entries {
				switch entry.Status {
				case m.Killed:
					killed++
					total++
				case m.Survived:
					total++
				case m.Skipped, m.Error:
					// Skipped/error entries are excluded from the score denominator.
				}
			}
		}

		return nil
	})
	if err != nil {
		return 0.0, err
	}

	if total == 0 {
		return 100.0, nil
	}

	return float64(killed) / float64(total), nil
}
