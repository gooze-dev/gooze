package controller

import (
	"github.com/spf13/cobra"
)

// SimpleUI implements UI using cobra Command's Println.
type SimpleUI struct {
	cmd *cobra.Command
}

// NewSimpleUI creates a new SimpleUI.
func NewSimpleUI(cmd *cobra.Command) *SimpleUI {
	return &SimpleUI{cmd: cmd}
}
