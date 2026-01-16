package controller

import (
	"io"
	"os"

	"github.com/spf13/cobra"
)

// NewUI creates a UI based on whether TTY mode is enabled.
// This is a factory function following the factory pattern.
// When useTTY is true, it returns a TUI (Bubble Tea).
// When useTTY is false, it returns a SimpleUI (plain text).
func NewUI(cmd *cobra.Command, useTTY bool) UI {
	if useTTY {
		return NewTUI(cmd.OutOrStdout())
	}

	return NewSimpleUI(cmd)
}

// IsTTY checks if the given writer is a terminal (TTY).
// Returns true if the output is an interactive terminal.
// Returns false if the output is redirected to a file or pipe.
func IsTTY(w io.Writer) bool {
	// Check if writer is a *os.File
	file, ok := w.(*os.File)
	if !ok {
		return false
	}

	// Get file info
	fileInfo, err := file.Stat()
	if err != nil {
		return false
	}

	// Check if it's a character device (terminal)
	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}
