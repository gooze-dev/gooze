// Package controller provides output adapters for displaying mutation testing results.
package controller

import (
	"context"

	"gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
)

// StartMode defines the mode of operation for the UI.
type StartMode int

// Available StartMode values.
const (
	ModeEstimate StartMode = iota
	ModeTest
)

// StartOption is a functional option for Start method.
type StartOption func(*StartConfig)

// StartConfig holds configuration for starting the UI.
type StartConfig struct {
	mode StartMode
}

// WithEstimateMode sets the UI to estimation mode.
func WithEstimateMode() StartOption {
	return func(c *StartConfig) {
		c.mode = ModeEstimate
	}
}

// WithTestMode sets the UI to test execution mode.
func WithTestMode() StartOption {
	return func(c *StartConfig) {
		c.mode = ModeTest
	}
}

// UI defines the interface for displaying mutation testing progress and results.
// Implementations can use different output methods (simple text, TUI, etc).
// It satisfies domain.Reporter so it can be injected into the workflow.
type UI interface {
	StartEstimate(ctx context.Context) error
	StartTest(ctx context.Context) error
	Close(ctx context.Context)
	Wait(ctx context.Context) // Wait for UI to finish (user closes it)
	DisplayEstimation(ctx context.Context, estimation domain.Estimation, err error) error
	DisplayConcurrencyInfo(ctx context.Context, threads int, shardIndex int, shardCount int)
	DisplayUpcomingTestsInfo(ctx context.Context, i int)
	DisplayStartingTestInfo(ctx context.Context, currentMutation m.Mutation, threadID int)
	DisplayCompletedTestInfo(ctx context.Context, currentMutation m.Mutation, mutationResult m.Result)
	DisplayMutationScore(ctx context.Context, score float64)
}
