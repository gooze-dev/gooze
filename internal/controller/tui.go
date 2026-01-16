package controller

import (
	"io"
)

// TUI implements UI using Bubble Tea for interactive display.
type TUI struct {
	output io.Writer
}

// NewTUI creates a new TUI.
func NewTUI(output io.Writer) *TUI {
	return &TUI{output: output}
}
