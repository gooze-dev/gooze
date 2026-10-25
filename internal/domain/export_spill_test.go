package domain

import (
	"testing"

	m "gooze.dev/pkg/gooze/internal/model"
	pkg "gooze.dev/pkg/gooze/pkg"
)

// SetReportSpill makes runs spill their reports to the spills create returns
// until t ends.
func SetReportSpill(t *testing.T, create func() (pkg.FileSpill[m.Report], error)) {
	t.Helper()

	previous := newReportSpill
	newReportSpill = create

	t.Cleanup(func() { newReportSpill = previous })
}
