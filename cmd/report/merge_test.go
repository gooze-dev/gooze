package report

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
	m "gooze.dev/pkg/gooze/internal/model"
)

func TestReportMerge_PassesTheDiffThresholds(t *testing.T) {
	viper.Set(testOutputKey, "/tmp/reports")
	viper.Set("threshold.max_survivors", 2)
	defer viper.Reset()

	wf := domainmocks.NewMockWorkflow(t)
	wf.On("Merge", mock.Anything, domain.MergeArgs{View: domain.ViewArgs{Gate: domain.GateAuto},
		Reports:        m.Path("/tmp/reports"),
		Thresholds:     domain.Thresholds{Key: "threshold", Total: 85, MaxSurvivors: 2},
		DiffThresholds: domain.Thresholds{Key: "threshold.diff", Total: 100, MaxSurvivors: 2},
	}).Return(nil)

	deps := Deps{Workflow: wf, Publisher: domainmocks.NewMockReportPublisher(t), OutputKey: testOutputKey}

	require.NoError(t, execReport(t, deps, "merge", "--threshold-total", "85", "--threshold-diff-total", "100"))
	wf.AssertExpectations(t)
}
