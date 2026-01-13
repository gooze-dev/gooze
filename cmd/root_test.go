package cmd

import (
	"bytes"
	"strings"
	"testing"
	"time"
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

func TestDefaultCommand(t *testing.T) {
	t.Run("default behavior runs mutations on basic example", func(t *testing.T) {
		cmd := newRootCmd()
		cmd.SetArgs([]string{"../examples/basic"})

		var out bytes.Buffer
		cmd.SetOut(&out)

		// Run with timeout since mutation testing can take time
		done := make(chan error, 1)
		go func() {
			done <- cmd.Execute()
		}()

		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("Execute error: %v", err)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("command timed out after 30s")
		}

		output := out.String()
		// Should show mutation testing output
		if !strings.Contains(output, "main.go") {
			t.Errorf("expected output to contain main.go, got: %s", output)
		}
		// Should have mutation results (killed or survived)
		if !strings.Contains(output, "killed") && !strings.Contains(output, "survived") {
			t.Errorf("expected output to contain mutation results, got: %s", output)
		}
	})

	t.Run("runs mutations on current directory by default", func(t *testing.T) {
		cmd := newRootCmd()
		cmd.SetArgs([]string{}) // No args = current directory

		var out bytes.Buffer
		cmd.SetOut(&out)

		done := make(chan error, 1)
		go func() {
			done <- cmd.Execute()
		}()

		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("Execute error: %v", err)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("command timed out after 30s")
		}

		output := out.String()
		// Should have some output
		if output == "" {
			t.Errorf("expected some output, got empty")
		}
	})

	t.Run("handles path with no mutations gracefully", func(t *testing.T) {
		cmd := newRootCmd()
		cmd.SetArgs([]string{"../examples/nofunc"})

		var out bytes.Buffer
		cmd.SetOut(&out)

		err := cmd.Execute()
		if err != nil {
			t.Fatalf("Execute error: %v", err)
		}

		output := out.String()
		// Should indicate no mutations found
		if !strings.Contains(output, "0") && !strings.Contains(output, "No") {
			t.Errorf("expected output to indicate no mutations, got: %s", output)
		}
	})
}
