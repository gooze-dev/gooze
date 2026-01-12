package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestListCommand(t *testing.T) {
	t.Run("list flag shows mutation counts", func(t *testing.T) {
		cmd := newRootCmd()
		cmd.SetArgs([]string{"--list", "../examples/basic"})

		var out bytes.Buffer
		cmd.SetOut(&out)

		err := cmd.Execute()
		if err != nil {
			t.Fatalf("Execute error: %v", err)
		}

		output := out.String()
		if !strings.Contains(output, "main.go") {
			t.Errorf("expected output to contain main.go, got: %s", output)
		}
		if !strings.Contains(output, "mutations") {
			t.Errorf("expected output to contain mutation count, got: %s", output)
		}
	})

	t.Run("short flag -l works", func(t *testing.T) {
		cmd := newRootCmd()
		cmd.SetArgs([]string{"-l", "../examples/basic"})

		var out bytes.Buffer
		cmd.SetOut(&out)

		err := cmd.Execute()
		if err != nil {
			t.Fatalf("Execute error: %v", err)
		}

		output := out.String()
		if !strings.Contains(output, "main.go") {
			t.Errorf("expected output to contain main.go, got: %s", output)
		}
	})

	t.Run("default argument uses current directory", func(t *testing.T) {
		cmd := newRootCmd()
		cmd.SetArgs([]string{"-l"})

		var out bytes.Buffer
		cmd.SetOut(&out)

		err := cmd.Execute()
		if err != nil {
			t.Fatalf("Execute error: %v", err)
		}

		// Should not error when no path specified
		output := out.String()
		if output == "" {
			t.Errorf("expected some output for current directory")
		}
	})

	t.Run("list with nonexistent path returns error", func(t *testing.T) {
		cmd := newRootCmd()
		cmd.SetArgs([]string{"-l", "/nonexistent/path"})

		err := cmd.Execute()
		if err == nil {
			t.Fatalf("expected error for nonexistent path")
		}
	})
}
