package adapter

import (
	m "github.com/mouse-blink/gooze/internal/model"
)

// ReportStore persists and retrieves mutation reports.
type ReportStore interface {
	SaveReports(path m.Path, reports []m.Report) error
	LoadReports(path m.Path) ([]m.Report, error)
}

type reportStore struct{}

// NewReportStore constructs a ReportStore implementation.
func NewReportStore() ReportStore {
	return &reportStore{}
}

func (rs *reportStore) SaveReports(_ m.Path, _ []m.Report) error {
	return nil
}

func (rs *reportStore) LoadReports(_ m.Path) ([]m.Report, error) {
	return nil, nil
}
