// Package controller provides output adapters for displaying mutation testing results.
package controller

// MutationEstimation holds estimation counts for different mutation types.
type MutationEstimation struct {
	Arithmetic int
	Boolean    int
}

// UI defines the interface for displaying source file lists.
// Implementations can use different output methods (simple text, TUI, etc).
type UI interface {
}
