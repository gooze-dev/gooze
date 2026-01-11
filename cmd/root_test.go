package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestListCommand(t *testing.T) {
	t.Run("list flag shows sources from single directory", func(t *testing.T) {
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
	})

	t.Run("default argument uses current directory", func(t *testing.T) {
		cmd := newRootCmd()
		cmd.SetArgs([]string{"--list"})

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

	t.Run("list shows only file paths", func(t *testing.T) {
		cmd := newRootCmd()
		cmd.SetArgs([]string{"--list", "../examples/mixed"})

		var out bytes.Buffer
		cmd.SetOut(&out)

		err := cmd.Execute()
		if err != nil {
			t.Fatalf("Execute error: %v", err)
		}

		output := out.String()
		// Should only show file paths, not scope details
		if !strings.Contains(output, "main.go") {
			t.Errorf("expected output to contain file path")
		}
		if strings.Contains(output, "Hash:") || strings.Contains(output, "Global") {
			t.Errorf("output should not contain scope details, got: %s", output)
		}
	})

	t.Run("list with nonexistent path returns error", func(t *testing.T) {
		cmd := newRootCmd()
		cmd.SetArgs([]string{"--list", "/nonexistent/path"})

		err := cmd.Execute()
		if err == nil {
			t.Fatalf("expected error for nonexistent path")
		}
	})
}
