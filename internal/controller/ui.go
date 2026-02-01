// Package controller provides output adapters for displaying mutation testing results.
package controller

import (
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
	Start(options ...StartOption) error
	Close()
	Wait() // Wait for UI to finish (user closes it)
	DisplayEstimation(mutations []m.Mutation, err error) error
	DisplayConcurrencyInfo(threads int, shardIndex int, shardCount int)
	DisplayUpcomingTestsInfo(i int)
	DisplayStartingTestInfo(currentMutation m.Mutation, threadID int)
	DisplayCompletedTestInfo(currentMutation m.Mutation, mutationResult m.Result)
	DisplayMutationScore(score float64)
}
