package domain

import (
	"testing"

	m "github.com/mouse-blink/gooze/internal/model"
)

// TestOrchestrator_NoTestFile verifies that when no test file is specified,
// the orchestrator reports the mutation as surviving with a clear message
// and without returning an error.
func TestOrchestrator_NoTestFile(t *testing.T) {
	// Adapters are not used in this code path, so nil is fine.
	orch := NewOrchestrator(nil, nil)

	source := m.Source{
		Origin: "dummy.go",
		Test:   "", // no test file
	}

	mutation := m.Mutation{}

	report, err := orch.TestMutation(source, mutation)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if report.Killed {
		t.Fatalf("expected mutation to survive when no test file is specified")
	}

	if report.Output != "no test file specified" {
		t.Fatalf("unexpected output: %q", report.Output)
	}
}
