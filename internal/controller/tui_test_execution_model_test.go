package controller

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func TestTestResult_FilterValue(t *testing.T) {
	result := testResult{id: "1", file: "a.go", typ: "bool", status: "killed"}
	got := result.FilterValue()
	if !strings.Contains(got, "1") || !strings.Contains(got, "a.go") || !strings.Contains(got, "bool") || !strings.Contains(got, "killed") {
		t.Fatalf("FilterValue() = %q", got)
	}
}

func TestAnimateScrollFileAndTruncateFile(t *testing.T) {
	if got := truncateFile("hello", 0); got != "" {
		t.Fatalf("truncateFile width 0 = %q", got)
	}
	if got := truncateFile("hello", 1); got != "…" {
		t.Fatalf("truncateFile width 1 = %q", got)
	}
	if got := truncateFile("hello", 10); got != "hello" {
		t.Fatalf("truncateFile no truncation = %q", got)
	}

	if got := animateScrollFile("abcdef", 3, 0); got != "ab…" {
		t.Fatalf("animateScrollFile pause = %q", got)
	}
	got := animateScrollFile("abcdef", 3, 10)
	if got == "ab…" || len([]rune(got)) != 3 {
		t.Fatalf("animateScrollFile scrolled = %q", got)
	}
}

func TestTestExecutionModel_HandleStartAndComplete(t *testing.T) {
	m := newTestExecutionModel()
	m = m.handleStartMutation(startMutationMsg{id: 5, thread: 1, kind: "arith", path: "path/a.go"})
	if m.currentFile != "path/a.go" || m.currentMutationID != "5" || m.currentType != "arith" || !m.rendered {
		t.Fatalf("handleStartMutation did not set state")
	}
	if m.threadFiles[1] != "path/a.go" || m.threadMutationIDs[1] != "5" {
		t.Fatalf("thread tracking not set")
	}

	m.totalMutations = 1
	m = m.handleCompletedMutation(completedMutationMsg{id: 5, kind: "arith", status: "killed"})
	if m.completedCount != 1 || m.progressPercent != 1 || !m.testingFinished {
		t.Fatalf("handleCompletedMutation did not complete progress")
	}
	if len(m.results) != 1 {
		t.Fatalf("results length = %d, want 1", len(m.results))
	}
	if len(m.resultsList.Items()) != 1 {
		t.Fatalf("results list items = %d, want 1", len(m.resultsList.Items()))
	}

	// when totalMutations is zero, progress should not update
	m.totalMutations = 0
	m = m.handleCompletedMutation(completedMutationMsg{id: 6, kind: "arith", status: "survived"})
	if m.progressPercent != 1 {
		t.Fatalf("progressPercent = %v, want 1", m.progressPercent)
	}
}

func TestTestExecutionModel_HandleKeyMsgAndTick(t *testing.T) {
	m := newTestExecutionModel()
	m.testingFinished = true
	m.rendered = true
	m.resultsList.SetItems([]list.Item{
		testResult{id: "1", file: "a.go", typ: "bool", status: "killed"},
		testResult{id: "2", file: "b.go", typ: "arith", status: "survived"},
	})

	m.lastSelected = -1
	updated, _ := m.handleKeyMsg(tea.KeyMsg{Type: tea.KeyDown})
	if updated.lastSelected == -1 {
		t.Fatalf("expected selection to update")
	}

	_, cmd := updated.handleKeyMsg(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatalf("expected quit cmd")
	}

	updated.animOffset = 0
	model, _ := updated.handleTickMsg(tickMsg(time.Now()))
	if model.animOffset != 1 {
		t.Fatalf("animOffset = %d, want 1", model.animOffset)
	}

	updated.testingFinished = false
	expectedOffset := updated.animOffset
	model, _ = updated.handleTickMsg(tickMsg(time.Now()))
	if model.animOffset != expectedOffset {
		t.Fatalf("animOffset changed unexpectedly")
	}

	fresh := newTestExecutionModel()
	_, cmd = fresh.handleKeyMsg(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if cmd != nil {
		t.Fatalf("expected nil cmd when not finished")
	}
}

func TestTestExecutionModel_WindowSizeAndViews(t *testing.T) {
	m := newTestExecutionModel()
	m = m.handleWindowSize(tea.WindowSizeMsg{Width: 10, Height: 5})
	if m.progressBar.Width != 20 {
		t.Fatalf("progress bar width = %d, want 20", m.progressBar.Width)
	}

	m = m.handleWindowSize(tea.WindowSizeMsg{Width: 80, Height: 30})
	if m.progressBar.Width != 72 {
		t.Fatalf("progress bar width = %d, want 72", m.progressBar.Width)
	}

	if got := m.View(); got != "Initializing test execution…\n" {
		t.Fatalf("View() before rendered finished = %q", got)
	}

	m.rendered = true

	m.threads = 1
	m.totalMutations = 2
	m.completedCount = 1
	progressView := m.viewProgress()
	if !strings.Contains(progressView, "Gooze Mutation Testing") {
		t.Fatalf("viewProgress missing title")
	}

	m.testingFinished = true
	m.results = []testResult{{status: "killed"}, {status: "error"}}
	resultsView := m.viewResults()
	if !strings.Contains(resultsView, "Gooze Test Results") {
		t.Fatalf("viewResults missing title")
	}

	box := m.renderResultsBox("6")
	if !strings.Contains(box, "ID") {
		t.Fatalf("renderResultsBox missing headers")
	}

	if got := m.countStatus("killed"); got != 1 {
		t.Fatalf("countStatus killed = %d, want 1", got)
	}

	// thread box with multiple threads and idle
	m.threads = 2
	m.threadFiles = map[int]string{0: "", 1: "path/to/long/file.go"}
	m.threadMutationIDs = map[int]string{1: "99"}
	threadBox := m.renderThreadBox("6")
	if !strings.Contains(threadBox, "Thread") {
		t.Fatalf("renderThreadBox missing thread label")
	}

	// Single thread mode (no thread labels)
	m.threads = 1
	m.threadFiles = map[int]string{0: "file.go"}
	m.threadMutationIDs = map[int]string{0: ""}
	threadBox = m.renderThreadBox("6")
	if strings.Contains(threadBox, "Thread") {
		t.Fatalf("single thread mode should not have Thread label")
	}
}

func TestTestResultDelegateStyles(t *testing.T) {
	delegate := testResultDelegate{}
	result := testResult{id: "1234", file: "path/to/file.go", typ: "bool", status: "custom"}

	_, _, _, _, display := delegate.getStylesAndFile(result, false, 10)
	if len([]rune(display)) == 0 {
		t.Fatalf("expected display file for unselected")
	}

	_, _, _, _, display = delegate.getStylesAndFile(result, true, 10)
	if len([]rune(display)) == 0 {
		t.Fatalf("expected display file for selected")
	}
}

func TestTestResultDelegate_Render(t *testing.T) {
	delegate := testResultDelegate{}
	items := []list.Item{testResult{id: "1", file: "short.go", typ: "bool", status: "killed"}}
	m := list.New(items, delegate, 60, 5)
	var buf strings.Builder
	delegate.Render(&buf, m, 0, items[0])
	if !strings.Contains(buf.String(), "short.go") {
		t.Fatalf("render output missing file")
	}

	// Render with bad item type should not panic
	buf.Reset()
	delegate.Render(&buf, m, 0, struct{ list.Item }{})

	// Test delegate methods
	if delegate.Height() != 1 {
		t.Fatalf("Height() = %d, want 1", delegate.Height())
	}
	if delegate.Spacing() != 0 {
		t.Fatalf("Spacing() = %d, want 0", delegate.Spacing())
	}
	if cmd := delegate.Update(nil, &m); cmd != nil {
		t.Fatalf("Update() returned cmd")
	}
}

func TestTestExecutionModel_UpdateSwitch(t *testing.T) {
	m := newTestExecutionModel()
	if cmd := m.Init(); cmd == nil {
		t.Fatalf("Init() returned nil cmd")
	}

	if view := m.View(); !strings.Contains(view, "Initializing") {
		t.Fatalf("View before start should show initializing")
	}

	_, _ = m.Update(tea.WindowSizeMsg{Width: 50, Height: 10})
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	_, _ = m.Update(tickMsg(time.Now()))
	model, _ := m.Update(startMutationMsg{id: 1, thread: 0, kind: "arith", path: "a.go"})
	m = model.(testExecutionModel)

	if view := m.View(); !strings.Contains(view, "Gooze Mutation Testing") {
		t.Fatalf("View after start should show testing")
	}

	_, _ = m.Update(completedMutationMsg{id: 1, kind: "arith", status: "killed"})
	_, _ = m.Update(concurrencyMsg{threads: 2, shardIndex: 1, shards: 3})
	_, _ = m.Update(upcomingMsg{count: 10})
	_, _ = m.Update(estimationMsg{})

	// Set filtering and test tick skip
	m.testingFinished = true
	m.resultsList.SetFilteringEnabled(true)
	_, _ = m.resultsList.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	_, cmd := m.handleTickMsg(tickMsg(time.Now()))
	_ = cmd
}
