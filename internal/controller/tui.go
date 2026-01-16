package controller

import (
	"io"
)

const (
	// ANSI color codes for zero values (dark gray, faint).
	grayColor  = "\033[2;90m" // Faint + dark gray
	resetColor = "\033[0m"    // Reset
)

// TUI implements UI using Bubble Tea for interactive display.
type TUI struct {
	output io.Writer
}

// NewTUI creates a new TUI.
func NewTUI(output io.Writer) *TUI {
	return &TUI{output: output}
}
