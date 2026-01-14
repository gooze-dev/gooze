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

	t.Run("list flag shows boolean mutation counts", func(t *testing.T) {
		cmd := newRootCmd()
		cmd.SetArgs([]string{"--list", "../examples/boolean"})

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
		// Must contain boolean mutation type reporting
		if !strings.Contains(output, "boolean") {
			t.Fatalf("expected output to contain 'boolean' mutation type, got: %s", output)
		}
		// Should NOT show 0 boolean mutations (examples/boolean has true/false literals)
		if strings.Contains(output, "0 boolean mutations") || !strings.Contains(output, "boolean mutations") {
			t.Fatalf("expected non-zero boolean mutations for examples/boolean, got: %s", output)
		}
	})

	t.Run("list flag shows both arithmetic and boolean mutations", func(t *testing.T) {
		cmd := newRootCmd()
		cmd.SetArgs([]string{"--list", "../examples/scopes"})

		var out bytes.Buffer
		cmd.SetOut(&out)

		err := cmd.Execute()
		if err != nil {
			t.Fatalf("Execute error: %v", err)
		}

		output := out.String()
		// scopes example has arithmetic operators
		if !strings.Contains(output, "arithmetic") {
			t.Errorf("expected output to contain 'arithmetic' mutation type, got: %s", output)
		}
		// Should also report boolean mutations
		if !strings.Contains(output, "boolean") {
			t.Errorf("expected output to contain 'boolean' mutation type, got: %s", output)
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
			// When running from cmd directory, it may fail to find go.mod for mutation testing
			// This is expected behavior, so we just check that something was attempted
			if err != nil && !strings.Contains(err.Error(), "go.mod not found") {
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

	t.Run("runs boolean mutations on boolean example", func(t *testing.T) {
		cmd := newRootCmd()
		cmd.SetArgs([]string{"../examples/boolean"})

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
		// Should show mutations were tested (not 0 mutations)
		// examples/boolean has 4 boolean literals, so should have at least 4 mutations
		if strings.Contains(output, ": 0 mutations") {
			t.Fatalf("expected non-zero mutations for boolean example, got: %s", output)
		}
		// Should have mutation results
		if !strings.Contains(output, "main.go") {
			t.Errorf("expected output to contain main.go, got: %s", output)
		}
	})
}
