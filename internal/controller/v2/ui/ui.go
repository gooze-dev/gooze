package ui

import (
	"context"

	m "gooze.dev/pkg/gooze/internal/model"
)

type UI interface {
	ShowEstimateProgress(ctx context.Context, events m.Events)
	ShowEstimateResults(ctx context.Context, events m.Events)
	ShowTestExecutionProgress(ctx context.Context, events m.Events)
	ShowTestExecutionResults(ctx context.Context, events m.Events)
}
