package controller

import (
	"bytes"
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	m "github.com/mouse-blink/gooze/internal/model"
)

type quitModel struct{}

func (m quitModel) Init() tea.Cmd { return tea.Quit }
func (m quitModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return m, tea.Quit
}
func (m quitModel) View() string { return "" }

func TestTUI_StartWithModel_WaitAndClose(t *testing.T) {
	var buf bytes.Buffer
	tui := NewTUI(&buf)

	if err := tui.startWithModel(quitModel{}); err != nil {
		t.Fatalf("startWithModel error = %v", err)
	}

	// send while running should go through program.Send
	tui.send(upcomingMsg{count: 2})

	waitDone := make(chan struct{})
	go func() {
		tui.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Wait() timed out")
	}

	closeDone := make(chan struct{})
	go func() {
		tui.Close()
		close(closeDone)
	}()

	select {
	case <-closeDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Close() timed out")
	}
}

func TestTUI_Send_And_EnsureStarted_NoPanic(t *testing.T) {
	var buf bytes.Buffer
	tui := NewTUI(&buf)

	// send before start should be no-op
	tui.send(upcomingMsg{count: 1})

	// ensureStarted should not re-start when already started
	tui.started = true
	tui.ensureStarted()
}

func TestTUI_DisplayCompletedTestInfo_WithDiff(t *testing.T) {
	var buf bytes.Buffer
	tui := NewTUI(&buf)

	diffCode := []byte("--- original\n+++ mutated\n@@ -1,1 +1,1 @@\n-\treturn 3 + 5\n+\treturn 3 - 5\n")

	mutation := m.Mutation{
		ID:       "hash-10",
		Type:     m.MutationArithmetic,
		Source:   m.Source{Origin: &m.File{ShortPath: "test.go", Hash: "hash1"}},
		DiffCode: diffCode,
	}

	survivedResult := m.Result{
		m.MutationArithmetic: []struct {
			MutationID string
			Status     m.TestStatus
			Err        error
		}{{MutationID: "hash-10", Status: m.Survived}},
	}

	// Start TUI in test mode
	if err := tui.Start(WithTestMode()); err != nil {
		t.Fatalf("Start error = %v", err)
	}

	// Test that DisplayCompletedTestInfo sends message with diff for survived mutation
	tui.DisplayCompletedTestInfo(mutation, survivedResult)

	// Verify it doesn't panic and the TUI is still functional
	tui.Close()
}

func TestTUI_DisplayCompletedTestInfo_WithoutDiff(t *testing.T) {
	var buf bytes.Buffer
	tui := NewTUI(&buf)

	mutation := m.Mutation{
		ID:     "hash-10",
		Type:   m.MutationArithmetic,
		Source: m.Source{Origin: &m.File{ShortPath: "test.go", Hash: "hash1"}},
		// No DiffCode
	}

	killedResult := m.Result{
		m.MutationArithmetic: []struct {
			MutationID string
			Status     m.TestStatus
			Err        error
		}{{MutationID: "hash-10", Status: m.Killed}},
	}

	// Start TUI in test mode
	if err := tui.Start(WithTestMode()); err != nil {
		t.Fatalf("Start error = %v", err)
	}

	// Test that DisplayCompletedTestInfo sends message without diff for killed mutation
	tui.DisplayCompletedTestInfo(mutation, killedResult)

	// Verify it doesn't panic and the TUI is still functional
	tui.Close()
}

func TestTUI_StartWithMouseCellMotion(t *testing.T) {
	var buf bytes.Buffer
	tui := NewTUI(&buf)

	// Test that TUI starts with mouse cell motion enabled (should not error)
	if err := tui.Start(); err != nil {
		t.Fatalf("Start error = %v", err)
	}

	tui.Close()
}

func TestTUI_MultipleClose(t *testing.T) {
	var buf bytes.Buffer
	tui := NewTUI(&buf)

	tui.Close()
	tui.Close() // Close again should be safe

	tui2 := NewTUI(&buf)
	tui2.Wait() // Wait without start should be no-op

	tui3 := NewTUI(&buf)
	tui3.Close() // Close without start should be no-op
}

func TestTUI_DisplayMethods_NoProgram(t *testing.T) {
	var buf bytes.Buffer
	tui := NewTUI(&buf)

	// Avoid starting Bubble Tea program in tests
	tui.started = true

	if err := tui.DisplayEstimation(nil, nil); err != nil {
		t.Fatalf("DisplayEstimation unexpected error = %v", err)
	}

	if err := tui.DisplayEstimation(nil, errSentinel); err == nil {
		t.Fatalf("DisplayEstimation expected error")
	}

	muts := []m.Mutation{
		{Source: m.Source{Origin: &m.File{ShortPath: "a.go", FullPath: "path/a.go", Hash: "hash-a"}}},
		{Source: m.Source{Origin: nil}},
	}
	if err := tui.DisplayEstimation(muts, nil); err != nil {
		t.Fatalf("DisplayEstimation with mutations error = %v", err)
	}

	tui.DisplayConcurencyInfo(2, 1, 3)
	tui.DusplayUpcomingTestsInfo(5)
	tui.DisplayStartingTestInfo(mutationWithOrigin(), 7)
	tui.DisplayStartingTestInfo(mutationWithoutOrigin(), 8)
	tui.DisplayCompletedTestInfo(mutationWithOrigin(), completedResult())
	tui.DisplayCompletedTestInfo(mutationWithoutOrigin(), mResultEmpty())
}

var errSentinel = errors.New("boom")

func mutationWithOrigin() m.Mutation {
	return m.Mutation{ID: "hash-1", Type: m.MutationArithmetic, Source: m.Source{Origin: &m.File{ShortPath: "a.go", FullPath: "path/a.go", Hash: "hash-a"}}}
}

func mutationWithoutOrigin() m.Mutation {
	return m.Mutation{ID: "hash-2", Type: m.MutationBoolean}
}

func completedResult() m.Result {
	return m.Result{
		m.MutationArithmetic: []struct {
			MutationID string
			Status     m.TestStatus
			Err        error
		}{{MutationID: "hash-1", Status: m.Killed}},
	}
}

func mResultEmpty() m.Result {
	return m.Result{}
}
