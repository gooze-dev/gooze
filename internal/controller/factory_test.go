package controller

import (
	"bytes"
	"os"
	"testing"

	"github.com/spf13/cobra"
)

func TestNewUI_TTYMode(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})

	ui := NewUI(cmd, true)

	if _, ok := ui.(*TUI); !ok {
		t.Errorf("NewUI(true) returned %T, want *TUI", ui)
	}
}

func TestNewUI_NonTTYMode(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})

	ui := NewUI(cmd, false)

	if _, ok := ui.(*SimpleUI); !ok {
		t.Errorf("NewUI(false) returned %T, want *SimpleUI", ui)
	}
}

func TestIsTTY_WithTerminal(t *testing.T) {
	// Note: This test checks if the function works correctly
	// It will return true if running in a real terminal, false otherwise
	result := IsTTY(os.Stdout)

	// We can't assert exact value as it depends on test environment
	// but we can verify it doesn't panic and returns a bool
	_ = result
}

func TestIsTTY_WithInvalidFile(t *testing.T) {
	file, err := os.CreateTemp("", "gooze-tty")
	if err != nil {
		t.Fatalf("CreateTemp error: %v", err)
	}
	file.Close()
	defer os.Remove(file.Name())

	if IsTTY(file) {
		t.Fatalf("IsTTY(invalid file) = true, want false")
	}
}

func TestIsTTY_WithCharDevice(t *testing.T) {
	file, err := os.Open("/dev/null")
	if err != nil {
		t.Skip("/dev/null not available")
	}
	defer file.Close()

	if !IsTTY(file) {
		t.Fatalf("IsTTY(/dev/null) = false, want true")
	}
}

func TestIsTTY_WithNonTerminal(t *testing.T) {
	// Create a buffer which is not a terminal
	var buf bytes.Buffer

	// IsTTY should handle non-file outputs gracefully
	// This should not panic
	result := IsTTY(&buf)

	// A buffer is never a TTY
	if result {
		t.Error("IsTTY(buffer) = true, want false")
	}
}
