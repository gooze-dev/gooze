package domain

import (
	"path/filepath"
	"testing"

	"github.com/mouse-blink/gooze/internal/adapter"

	m "github.com/mouse-blink/gooze/internal/model"
)

func TestEstimateMutations(t *testing.T) {
	t.Run("estimates mutations for single source file", func(t *testing.T) {
		basicPath := filepath.Join("..", "..", "examples", "basic", "main.go")
		source := loadSourceFromFile(t, basicPath)

		wf := NewWorkflow(adapter.NewLocalSourceFSAdapter(), adapter.NewLocalGoFileAdapter(), adapter.NewLocalTestRunnerAdapter())
		count, err := wf.EstimateMutations(source, m.MutationArithmetic)
		if err != nil {
			t.Fatalf("EstimateMutations failed: %v", err)
		}

		// examples/basic has one + operator, which generates 4 mutations
		if count != 4 {
			t.Errorf("expected 4 mutations, got %d", count)
		}
	})

	t.Run("estimates mutations for scopes example", func(t *testing.T) {
		scopesPath := filepath.Join("..", "..", "examples", "scopes", "main.go")
		source := loadSourceFromFile(t, scopesPath)

		wf := NewWorkflow(adapter.NewLocalSourceFSAdapter(), adapter.NewLocalGoFileAdapter(), adapter.NewLocalTestRunnerAdapter())
		count, err := wf.EstimateMutations(source, m.MutationArithmetic)
		if err != nil {
			t.Fatalf("EstimateMutations failed: %v", err)
		}

		// scopes has 2 operators (+ and -) × 4 mutations each = 8
		if count < 8 {
			t.Errorf("expected at least 8 mutations, got %d", count)
		}
	})

	t.Run("returns zero for files without arithmetic operators", func(t *testing.T) {
		emptyPath := filepath.Join("..", "..", "examples", "empty", "main.go")
		source := loadSourceFromFile(t, emptyPath)

		wf := NewWorkflow(adapter.NewLocalSourceFSAdapter(), adapter.NewLocalGoFileAdapter(), adapter.NewLocalTestRunnerAdapter())
		count, err := wf.EstimateMutations(source, m.MutationArithmetic)
		if err != nil {
			t.Fatalf("EstimateMutations failed: %v", err)
		}

		if count != 0 {
			t.Errorf("expected 0 mutations for file without arithmetic, got %d", count)
		}
	})

	t.Run("handles errors in source files gracefully", func(t *testing.T) {
		// Create a source with invalid path
		invalidSource := m.Source{
			Hash:   "invalid",
			Origin: m.Path("/nonexistent/file.go"),
			Scopes: []m.CodeScope{},
		}

		wf := NewWorkflow(adapter.NewLocalSourceFSAdapter(), adapter.NewLocalGoFileAdapter(), adapter.NewLocalTestRunnerAdapter())
		_, err := wf.EstimateMutations(invalidSource, m.MutationArithmetic)
		if err == nil {
			t.Error("expected error for invalid source, got nil")
		}
	})

	t.Run("estimate matches actual mutations generated", func(t *testing.T) {
		basicPath := filepath.Join("..", "..", "examples", "basic", "main.go")
		source := loadSourceFromFile(t, basicPath)

		wf := NewWorkflow(adapter.NewLocalSourceFSAdapter(), adapter.NewLocalGoFileAdapter(), adapter.NewLocalTestRunnerAdapter())

		// Get estimate
		estimated, err := wf.EstimateMutations(source, m.MutationArithmetic)
		if err != nil {
			t.Fatalf("EstimateMutations failed: %v", err)
		}

		// Generate actual mutations
		mutations, err := wf.GenerateMutations(source, m.MutationArithmetic)
		if err != nil {
			t.Fatalf("GenerateMutations failed: %v", err)
		}

		if estimated != len(mutations) {
			t.Errorf("estimate (%d) does not match actual mutations (%d)", estimated, len(mutations))
		}
	})

	t.Run("returns error for unsupported mutation type", func(t *testing.T) {
		basicPath := filepath.Join("..", "..", "examples", "basic", "main.go")
		source := loadSourceFromFile(t, basicPath)

		wf := NewWorkflow(adapter.NewLocalSourceFSAdapter(), adapter.NewLocalGoFileAdapter(), adapter.NewLocalTestRunnerAdapter())
		_, err := wf.EstimateMutations(source, m.MutationType("unsupported"))
		if err == nil {
			t.Error("expected error for unsupported mutation type, got nil")
		}
	})
}
