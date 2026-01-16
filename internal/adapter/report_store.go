package adapter

import (
	m "github.com/mouse-blink/gooze/internal/model"
)

type ReportStore interface {
	SaveReports(path m.Path, reports []m.ReportV2) error
	LoadReports(path m.Path) ([]m.ReportV2, error)
}
