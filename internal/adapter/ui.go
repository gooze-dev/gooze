// Package adapter provides output adapters for displaying mutation testing results.
package adapter

import (
	m "github.com/mouse-blink/gooze/internal/model"
)

// UI defines the interface for displaying source file lists.
// Implementations can use different output methods (simple text, TUI, etc).
type UI interface {
	// ShowNotImplemented displays a "not implemented" message with file count.
	ShowNotImplemented(count int) error
	// DisplayMutationEstimations displays pre-calculated mutation estimations.
	DisplayMutationEstimations(estimations map[m.Path]int) error
}
