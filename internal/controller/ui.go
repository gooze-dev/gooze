// Package controller provides output adapters for displaying mutation testing results.
package controller

import (
	"context"

	m "gooze.dev/pkg/gooze/internal/model"
)

// MutationEstimation holds estimation counts for different mutation types.
type MutationEstimation struct {
	Arithmetic int
	Boolean    int
}

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

// UI defines the interface for displaying source file lists.
// Implementations can use different output methods (simple text, TUI, etc).
type UI interface {
	Start(ctx context.Context, options ...StartOption) error
	Close(ctx context.Context)
	Wait(ctx context.Context) // Wait for UI to finish (user closes it)
	DisplayEstimation(ctx context.Context, mutations []m.Mutation, err error) error
	DisplayConcurrencyInfo(ctx context.Context, threads int, shardIndex int, shardCount int)
	DisplayUpcomingTestsInfo(ctx context.Context, i int)
	DisplayStartingTestInfo(ctx context.Context, currentMutation m.Mutation, threadID int)
	DisplayCompletedTestInfo(ctx context.Context, currentMutation m.Mutation, mutationResult m.Result)
	DisplayMutationScore(ctx context.Context, score float64)
}
