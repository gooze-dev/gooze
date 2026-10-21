package adapter

import "gooze.dev/pkg/gooze/internal/domain"

var (
	_ domain.SourceFSAdapter   = (*LocalSourceFSAdapter)(nil)
	_ domain.GoFileAdapter     = (*LocalGoFileAdapter)(nil)
	_ domain.TestRunnerAdapter = (*LocalTestRunnerAdapter)(nil)
	_ domain.ReportStore       = (*LocalReportStore)(nil)
	_ domain.OCIRegistry       = (*ORASRegistry)(nil)
	_ domain.Differ            = (*UnifiedDiffer)(nil)
	_ domain.ReleaseChecker    = (*ModuleProxyReleaseChecker)(nil)
	_ domain.ChangeDetector    = (*GitChangeDetector)(nil)
)
