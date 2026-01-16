package adapter

import (
	m "github.com/mouse-blink/gooze/internal/model"
)

type ReportStore interface {
	SaveReports(path m.Path, reports []m.Report) error
	LoadReports(path m.Path) ([]m.Report, error)
}

type reportStore struct{}

func NewReportStore() ReportStore {
	return &reportStore{}
}

func (rs *reportStore) SaveReports(path m.Path, reports []m.Report) error {
	return nil
}

func (rs *reportStore) LoadReports(path m.Path) ([]m.Report, error) {
	return nil, nil
}
