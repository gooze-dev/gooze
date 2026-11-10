package controller

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"gooze.dev/pkg/gooze/internal/domain"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

func TestTestResult_FilterValue(t *testing.T) {
	result := testResult{id: "1", file: "a.go", typ: "bool", status: "killed"}
	got := result.FilterValue()
	if !strings.Contains(got, "1") || !strings.Contains(got, "a.go") || !strings.Contains(got, "bool") || !strings.Contains(got, "killed") {
		t.Fatalf("FilterValue() = %q", got)
	}
}

func TestAnimateScrollFileAndTruncateFile(t *testing.T) {
	if got := truncateToWidth("hello", 0); got != "" {
		t.Fatalf("truncateFile width 0 = %q", got)
	}
	if got := truncateToWidth("hello", 1); got != "…" {
		t.Fatalf("truncateFile width 1 = %q", got)
	}
	if got := truncateToWidth("hello", 10); got != "hello" {
		t.Fatalf("truncateFile no truncation = %q", got)
	}

	if got := animateScroll("abcdef", 3, 0); got != "ab…" {
		t.Fatalf("animateScrollFile pause = %q", got)
	}
	got := animateScroll("abcdef", 3, 10)
	if got == "ab…" || len([]rune(got)) != 3 {
		t.Fatalf("animateScrollFile scrolled = %q", got)
	}
}

func TestTestExecutionModel_HandleStartAndComplete(t *testing.T) {
	m := newTestExecutionModel()
	m = m.handleStartMutation(startMutationMsg{id: "hash-5", thread: 1, kind: "arith", fileHash: "hash-a", displayPath: "path/a.go"})
	if m.currentFile != "path/a.go" || m.currentMutationID != "hash-5" || m.currentType != "arith" || !m.rendered {
		t.Fatalf("handleStartMutation did not set state")
	}
	if m.threadFiles[1] != "path/a.go" || m.threadMutationIDs[1] != "hash-5" {
		t.Fatalf("thread tracking not set")
	}

	m.totalMutations = 1
	m = m.handleCompletedMutation(completedMutationMsg{id: "hash-5", kind: "arith", fileHash: "hash-a", displayPath: "path/a.go", status: "killed"})
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
	m = m.handleCompletedMutation(completedMutationMsg{id: "hash-6", kind: "arith", fileHash: "hash-b", displayPath: "path/b.go", status: "survived"})
	if m.progressPercent != 1 {
		t.Fatalf("progressPercent = %v, want 1", m.progressPercent)
	}
}

func TestTestExecutionModel_CountingPhase(t *testing.T) {
	m := newTestExecutionModel()

	// Concurrency info arrives first (before the count pass finishes).
	m = m.handleConcurrency(concurrencyMsg{threads: 2, shardIndex: 0, shards: 1})
	if !m.rendered || !m.counting {
		t.Fatalf("expected rendered and counting after concurrency")
	}

	if got := m.View(); !strings.Contains(got, "Counting mutations") {
		t.Fatalf("View during count pass = %q, want counting message", got)
	}

	// Once the total is known, counting ends and the progress bar takes over.
	m = m.handleUpcoming(upcomingMsg{count: 5})
	if m.counting || m.totalMutations != 5 {
		t.Fatalf("expected counting=false and total=5 after upcoming")
	}

	if got := m.View(); strings.Contains(got, "Counting mutations") {
		t.Fatalf("View after upcoming should not show counting message")
	}
}

func TestTestExecutionModel_CompletionClearsThread(t *testing.T) {
	m := newTestExecutionModel()
	m.threads = 2
	m.totalMutations = 2

	m = m.handleStartMutation(startMutationMsg{id: "aaaa", thread: 0, kind: "arith", displayPath: "a.go"})
	m = m.handleStartMutation(startMutationMsg{id: "bbbb", thread: 1, kind: "bool", displayPath: "b.go"})

	// Completing thread 0's mutation frees that thread; thread 1 stays busy.
	m = m.handleCompletedMutation(completedMutationMsg{id: "aaaa", kind: "arith", displayPath: "a.go", status: "killed"})

	if _, ok := m.threadMutationIDs[0]; ok {
		t.Fatalf("thread 0 should be idle after its mutation completed")
	}

	if m.threadMutationIDs[1] != "bbbb" {
		t.Fatalf("thread 1 should still be running its mutation")
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

func TestTestExecutionModel_ToggleDiff(t *testing.T) {
	m := newTestExecutionModel()
	m.testingFinished = true
	m.rendered = true
	m.resultsList.SetItems([]list.Item{
		testResult{id: "1", file: "a.go", typ: "bool", status: "survived", diff: "--- a\n+++ b\n"},
	})
	m.resultsList.Select(0)

	updated, _ := m.handleKeyMsg(tea.KeyMsg{Type: tea.KeyEnter})
	if !updated.showDiff || updated.selectedDiff == "" {
		t.Fatalf("expected diff to be shown on enter")
	}

	updated, _ = updated.handleKeyMsg(tea.KeyMsg{Type: tea.KeyEnter})
	if updated.showDiff || updated.selectedDiff != "" {
		t.Fatalf("expected diff to be hidden on second enter")
	}

	mouse := tea.MouseMsg(tea.MouseEvent{Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})
	updated, _ = updated.handleMouseMsg(mouse)
	if !updated.showDiff {
		t.Fatalf("expected diff to be shown on mouse click")
	}

	updated.resultsList.SetItems([]list.Item{
		testResult{id: "1", file: "a.go", typ: "bool", status: "survived", diff: ""},
	})
	updated.resultsList.Select(0)
	updated.showDiff = false
	updated.selectedDiff = ""
	updated.selectedDiffPath = ""
	updated, _ = updated.handleKeyMsg(tea.KeyMsg{Type: tea.KeyEnter})
	if updated.showDiff {
		t.Fatalf("expected diff to remain hidden when empty")
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

	box := m.renderResultsBox()
	if !strings.Contains(box, "ID") {
		t.Fatalf("renderResultsBox missing headers")
	}

	if got := m.countStatus("killed"); got != 1 {
		t.Fatalf("countStatus killed = %d, want 1", got)
	}

	// thread box with multiple threads and idle
	m.threads = 2
	m.threadFiles = map[int]string{0: "", 1: "path/to/long/file.go"}
	m.threadMutationIDs = map[int]string{1: "abcd5678"}
	threadBox := m.renderThreadBox()
	if !strings.Contains(threadBox, "Thread") {
		t.Fatalf("renderThreadBox missing thread label")
	}

	// Single thread mode (no thread labels)
	m.threads = 1
	m.threadFiles = map[int]string{0: "file.go"}
	m.threadMutationIDs = map[int]string{0: ""}
	threadBox = m.renderThreadBox()
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
	items := []list.Item{testResult{id: "abcd1234", file: "short.go", typ: "bool", status: "killed"}}
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
	model, _ := m.Update(startMutationMsg{id: "hash-1", thread: 0, kind: "arith", fileHash: "hash-a", displayPath: "a.go"})
	m = model.(testExecutionModel)

	if view := m.View(); !strings.Contains(view, "Gooze Mutation Testing") {
		t.Fatalf("View after start should show testing")
	}

	_, _ = m.Update(completedMutationMsg{id: "hash-1", kind: "arith", fileHash: "hash-test", displayPath: "test.go", status: "killed"})
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

func TestTestExecutionModel_ParallelMutationTracking(t *testing.T) {
	// This test simulates parallel execution with 4 threads
	// and verifies that each thread correctly tracks its own mutation ID
	m := newTestExecutionModel()
	m.threads = 4
	m.totalMutations = 4
	m.threadFiles = make(map[int]string)
	m.threadMutationIDs = make(map[int]string)

	// Simulate 4 mutations starting in parallel on different threads
	m = m.handleStartMutation(startMutationMsg{id: "hash-0", thread: 0, kind: "arith", fileHash: "hash-1", displayPath: "file1.go"})
	m = m.handleStartMutation(startMutationMsg{id: "hash-1", thread: 1, kind: "bool", fileHash: "hash-2", displayPath: "file2.go"})
	m = m.handleStartMutation(startMutationMsg{id: "hash-2", thread: 2, kind: "comp", fileHash: "hash-3", displayPath: "file3.go"})
	m = m.handleStartMutation(startMutationMsg{id: "hash-3", thread: 3, kind: "logic", fileHash: "hash-4", displayPath: "file4.go"})

	// Verify each thread is tracking the correct mutation ID
	if m.threadMutationIDs[0] != "hash-0" {
		t.Fatalf("thread 0 mutation ID = %q, want \"hash-0\"", m.threadMutationIDs[0])
	}
	if m.threadMutationIDs[1] != "hash-1" {
		t.Fatalf("thread 1 mutation ID = %q, want \"hash-1\"", m.threadMutationIDs[1])
	}
	if m.threadMutationIDs[2] != "hash-2" {
		t.Fatalf("thread 2 mutation ID = %q, want \"hash-2\"", m.threadMutationIDs[2])
	}
	if m.threadMutationIDs[3] != "hash-3" {
		t.Fatalf("thread 3 mutation ID = %q, want \"hash-3\"", m.threadMutationIDs[3])
	}

	// Simulate mutations completing in a different order (2, 0, 3, 1)
	m = m.handleCompletedMutation(completedMutationMsg{id: "hash-2", kind: "comp", fileHash: "hash-3", displayPath: "file3.go", status: "killed"})
	m = m.handleCompletedMutation(completedMutationMsg{id: "hash-0", kind: "arith", fileHash: "hash-1", displayPath: "file1.go", status: "survived"})
	m = m.handleCompletedMutation(completedMutationMsg{id: "hash-3", kind: "logic", fileHash: "hash-4", displayPath: "file4.go", status: "killed"})
	m = m.handleCompletedMutation(completedMutationMsg{id: "hash-1", kind: "bool", fileHash: "hash-2", displayPath: "file2.go", status: "killed"})

	// Verify all results were recorded with correct IDs
	if len(m.results) != 4 {
		t.Fatalf("results length = %d, want 4", len(m.results))
	}

	// Check that each result has the correct ID (they should be in completion order)
	expectedIDs := []string{"hash", "hash", "hash", "hash"}
	for i, expected := range expectedIDs {
		if m.results[i].id != expected {
			t.Fatalf("result[%d].id = %q, want %q", i, m.results[i].id, expected)
		}
	}

	// Verify progress tracking
	if m.completedCount != 4 {
		t.Fatalf("completedCount = %d, want 4", m.completedCount)
	}
	if !m.testingFinished {
		t.Fatal("testingFinished should be true")
	}
	if m.progressPercent != 1.0 {
		t.Fatalf("progressPercent = %v, want 1.0", m.progressPercent)
	}
}

func TestTestExecutionModel_CompletedMutationsAppendToListInOrder(t *testing.T) {
	m := newTestExecutionModel()
	m.totalMutations = 3

	for i, file := range []string{"a.go", "b.go", "c.go"} {
		m = m.handleCompletedMutation(completedMutationMsg{id: fmt.Sprintf("hash-%d", i), kind: "arith", displayPath: file, status: "survived"})
	}

	items := m.resultsList.Items()
	if len(items) != 3 {
		t.Fatalf("expected 3 list items, got %d", len(items))
	}

	for i, want := range []string{"a.go", "b.go", "c.go"} {
		if got := items[i].(testResult).file; got != want {
			t.Fatalf("item %d file = %q, want %q", i, got, want)
		}
	}
}

func TestTestExecutionModel_DiffMaxLines(t *testing.T) {
	cases := map[int]int{0: 6, 18: 6, 21: 7, 45: 15, 60: 20, 300: 20}

	for height, want := range cases {
		m := newTestExecutionModel()
		m.height = height

		if got := m.diffMaxLines(); got != want {
			t.Fatalf("diffMaxLines() with height %d = %d, want %d", height, got, want)
		}
	}
}

func TestTestResultDelegate_RenderEveryStatusOnOneLine(t *testing.T) {
	const width = 80

	for status := rpt.Killed; status.String() != "unknown"; status++ {
		for _, selected := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/selected=%v", status, selected), func(t *testing.T) {
				delegate := testResultDelegate{}
				items := []list.Item{testResult{id: "abcd1234", file: "internal/domain/workflow.go", typ: "comparison", status: status.String()}}
				m := list.New(items, delegate, width, 5)

				if !selected {
					m.Select(1) // out of range: row 0 renders unselected
				}

				var buf strings.Builder
				delegate.Render(&buf, m, 0, items[0])
				row := buf.String()

				if strings.Contains(row, "\n") {
					t.Fatalf("row wraps onto a second line:\n%s", row)
				}

				if !strings.Contains(row, status.String()) {
					t.Fatalf("row %q lacks the full status %q", row, status)
				}

				if got := lipgloss.Width(row); got > width {
					t.Fatalf("row is %d cells wide, want at most %d: %q", got, width, row)
				}
			})
		}
	}
}

func TestStatusColumnFitsLongestLabel(t *testing.T) {
	if statusColumnWidth != len("not_covered")+2 {
		t.Fatalf("statusColumnWidth = %d, want %d", statusColumnWidth, len("not_covered")+2)
	}
}

func TestTestResultDelegate_RenderColumns(t *testing.T) {
	file := "internal/controller/some/deeply/nested/package/and/more/tui_test_execution_model.go"

	// The width minus id (6), status (13), type (12) and spacing (12) is left
	// for the file.
	for _, tt := range []struct{ width, fileWidth int }{{80, 37}, {100, 57}} {
		t.Run(fmt.Sprint(tt.width), func(t *testing.T) {
			delegate := testResultDelegate{}
			items := []list.Item{testResult{id: "abcd1234", file: file, typ: "comparison", status: "not_covered"}}
			m := list.New(items, delegate, tt.width, 5)
			m.Select(1) // out of range: row 0 renders unselected

			var buf strings.Builder
			delegate.Render(&buf, m, 0, items[0])

			want := "abcd    not_covered    comparison    " + truncateToWidth(file, tt.fileWidth)
			if got := buf.String(); got != want {
				t.Fatalf("row =\n%q\nwant\n%q", got, want)
			}
		})
	}
}

func TestTestResultDelegate_BoldIDAndStatus(t *testing.T) {
	delegate := testResultDelegate{}
	result := testResult{id: "abcd", file: "a.go", typ: "comparison", status: "killed"}

	for _, selected := range []bool{false, true} {
		idStyle, statusStyle, _, _, _ := delegate.getStylesAndFile(result, selected, 20)
		if !idStyle.GetBold() || !statusStyle.GetBold() {
			t.Fatalf("selected=%v: id bold %v, status bold %v; want both bold", selected, idStyle.GetBold(), statusStyle.GetBold())
		}
	}
}

func TestDiffLineStyle(t *testing.T) {
	tests := []struct {
		line   string
		fg, bg lipgloss.TerminalColor
		bold   bool
	}{
		{"+++ mutated", themeToxic, themeToxicFill, true},
		{"--- original", themeAccent, lipgloss.NoColor{}, true},
		{"@@ -1 +1 @@", themeMuted, lipgloss.NoColor{}, true},
		{"+	return a - b", themeToxic, themeToxicFill, false},
		{"-	return a + b", themeAccent, lipgloss.NoColor{}, false},
		{"   ", themeMuted, lipgloss.NoColor{}, false},
		{" 	x := 1", themeText, lipgloss.NoColor{}, false},
	}

	for _, tt := range tests {
		style := diffLineStyle(tt.line)
		if got := style.GetForeground(); got != tt.fg {
			t.Errorf("diffLineStyle(%q) foreground = %+v, want %+v", tt.line, got, tt.fg)
		}

		if got := style.GetBackground(); got != tt.bg {
			t.Errorf("diffLineStyle(%q) background = %+v, want %+v", tt.line, got, tt.bg)
		}

		if got := style.GetBold(); got != tt.bold {
			t.Errorf("diffLineStyle(%q) bold = %v, want %v", tt.line, got, tt.bold)
		}
	}
}

func TestTestResultDelegate_ThemeColors(t *testing.T) {
	delegate := testResultDelegate{}
	result := testResult{id: "abcd", file: "a.go", typ: "comparison", status: "survived"}

	idStyle, statusStyle, typeStyle, fileStyle, _ := delegate.getStylesAndFile(result, false, 20)
	if idStyle.GetForeground() != themeText || typeStyle.GetForeground() != themeMuted ||
		fileStyle.GetForeground() != themeAccent || statusStyle.GetForeground() != themeOrange {
		t.Fatalf("unselected row colors: id %+v type %+v file %+v status %+v",
			idStyle.GetForeground(), typeStyle.GetForeground(), fileStyle.GetForeground(), statusStyle.GetForeground())
	}

	idStyle, _, _, fileStyle, _ = delegate.getStylesAndFile(result, true, 20)
	if idStyle.GetBackground() != themeHighlight || fileStyle.GetForeground() != themeHighlightText {
		t.Fatalf("selected row: id bg %+v file fg %+v, want highlight", idStyle.GetBackground(), fileStyle.GetForeground())
	}
}

var updateGolden = flag.Bool("update", false, "rewrite the TUI golden files in testdata/tui")

// assertGolden compares a rendered view with testdata/tui/<name>.golden; run
// the tests with -update to rewrite the files after an intended layout change.
func assertGolden(t *testing.T, name, got string) {
	t.Helper()

	path := filepath.Join("testdata", "tui", name+".golden")
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run with -update to create it)", path, err)
	}

	if got != string(want) {
		t.Fatalf("%s differs from %s:\n--- got\n%s\n--- want\n%s", name, path, got, want)
	}
}

// sizedModel returns a model laid out for a width x height terminal.
func sizedModel(width, height int) testExecutionModel {
	m := newTestExecutionModel()

	return m.handleWindowSize(tea.WindowSizeMsg{Width: width, Height: height})
}

const longPath = "internal/adapter/some/deeply/nested/package/with_a_long_file_name.go"

func TestTestExecutionModel_ViewProgressGolden(t *testing.T) {
	counting := sizedModel(60, 20).handleConcurrency(concurrencyMsg{threads: 2, shardIndex: 1, shards: 3})
	assertGolden(t, "progress_counting", counting.View())

	single := sizedModel(60, 20).handleConcurrency(concurrencyMsg{threads: 1, shardIndex: 0, shards: 1})
	single = single.handleUpcoming(upcomingMsg{count: 10})
	single = single.handleStartMutation(startMutationMsg{id: "abcd1234", thread: 0, kind: "arithmetic", displayPath: longPath})
	single.completedCount = 3
	single.progressPercent = 0.3
	assertGolden(t, "progress_one_thread", single.View())

	threads := sizedModel(60, 20).handleConcurrency(concurrencyMsg{threads: 11, shardIndex: 0, shards: 1})
	threads = threads.handleUpcoming(upcomingMsg{count: 10})
	threads = threads.handleStartMutation(startMutationMsg{id: "abcd1234", thread: 0, kind: "arithmetic", displayPath: longPath})
	threads.threadFiles[10] = "calc.go"
	assertGolden(t, "progress_threads", threads.View())
	assertGolden(t, "thread_box", threads.renderThreadBox())

	ten := sizedModel(60, 20).handleConcurrency(concurrencyMsg{threads: 10, shardIndex: 0, shards: 1})
	ten = ten.handleUpcoming(upcomingMsg{count: 10})
	ten = ten.handleStartMutation(startMutationMsg{id: "abcd1234", thread: 0, kind: "arithmetic", displayPath: longPath})
	assertGolden(t, "thread_box_ten", ten.renderThreadBox())

	narrow := sizedModel(30, 20).handleConcurrency(concurrencyMsg{threads: 10, shardIndex: 0, shards: 1})
	narrow = narrow.handleUpcoming(upcomingMsg{count: 10})
	narrow = narrow.handleStartMutation(startMutationMsg{id: "abcd1234", thread: 0, kind: "arithmetic", displayPath: longPath})
	assertGolden(t, "progress_narrow", narrow.View())
}

// finishedModel returns a results view of three mutants with the middle one
// selected and its file name scrolled.
func finishedModel(width, height int) testExecutionModel {
	m := sizedModel(width, height).handleUpcoming(upcomingMsg{count: 3})
	for i, status := range []string{"killed", "survived", "error"} {
		m = m.handleCompletedMutation(completedMutationMsg{
			id: fmt.Sprintf("%d%d%d%d", i, i, i, i), kind: "arithmetic", displayPath: longPath, status: status,
			diff: []byte("--- a/calc.go\n+++ b/calc.go\n@@ -1 +1 @@\n-\treturn a + b\n+\treturn a - b\n"),
		})
	}

	m.resultsList.Select(1)
	m.lastSelected = 1
	m.delegate.offset = 9
	m.resultsList.SetDelegate(m.delegate)

	return m
}

func TestTestExecutionModel_ViewResultsGolden(t *testing.T) {
	plain := finishedModel(70, 22)
	assertGolden(t, "results_plain", plain.View())

	scored := finishedModel(70, 22)
	scored = scored.handleHeaderMsg(mutationScoreMsg{score: domain.MutationScore{Killed: 1, Survived: 1, ReportsDir: ".gooze-reports/run"}})
	assertGolden(t, "results_scored", scored.View())

	noHint := finishedModel(70, 22)
	noHint = noHint.handleHeaderMsg(mutationScoreMsg{score: domain.MutationScore{Killed: 1, Survived: 1}})
	assertGolden(t, "results_scored_no_hint", noHint.View())

	unscored := finishedModel(70, 22)
	unscored.mutationScore = domain.MutationScore{ReportsDir: ".gooze-reports/run"}
	assertGolden(t, "results_unscored_with_dir", unscored.View())

	withDiff := finishedModel(70, 30)
	withDiff.toggleSelectedDiff()
	assertGolden(t, "results_diff", withDiff.View())

	long := finishedModel(70, 18)
	long.showDiff = true
	long.selectedDiff = strings.TrimSpace(strings.Repeat("+\tline\n", 12))
	assertGolden(t, "results_diff_truncated", long.View())

	small := finishedModel(50, 10)
	assertGolden(t, "results_small", small.View())
}

func TestTestExecutionModel_RenderDiffBoxNarrow(t *testing.T) {
	m := newTestExecutionModel()
	m.showDiff = true
	m.selectedDiff = "-\treturn a + b\n+\treturn a - b"

	box, height := m.renderDiffBox(8)
	assertGolden(t, "diff_box_narrow", box)

	if height != lipgloss.Height(box) || height != 9 {
		t.Fatalf("renderDiffBox height = %d, want 9, the wrapped box's height", height)
	}

	m.showDiff = false
	if box, height := m.renderDiffBox(40); box != "" || height != 0 {
		t.Fatalf("hidden diff box = %q, %d; want empty, 0", box, height)
	}

	m.showDiff = true
	m.selectedDiff = "  "
	if box, height := m.renderDiffBox(40); box != "" || height != 0 {
		t.Fatalf("blank diff box = %q, %d; want empty, 0", box, height)
	}
}

func TestTestExecutionModel_DiffBoxHeight(t *testing.T) {
	m := newTestExecutionModel()
	m.height = 18 // diffMaxLines 6
	m.selectedDiff = "a\nb"

	if got := m.diffBoxHeight(); got != 0 {
		t.Fatalf("hidden diffBoxHeight = %d, want 0", got)
	}

	m.showDiff = true
	if got := m.diffBoxHeight(); got != 5 {
		t.Fatalf("diffBoxHeight of 2 lines = %d, want 5", got)
	}

	m.selectedDiff = strings.Repeat("x\n", 6) + "x"
	if got := m.diffBoxHeight(); got != 9 {
		t.Fatalf("diffBoxHeight of 7 lines capped at 6 = %d, want 9", got)
	}

	m.selectedDiff = strings.Repeat("x\n", 5) + "x"
	if got := m.diffBoxHeight(); got != 9 {
		t.Fatalf("diffBoxHeight of 6 lines = %d, want 9", got)
	}

	m.selectedDiff = " \n "
	if got := m.diffBoxHeight(); got != 0 {
		t.Fatalf("blank diffBoxHeight = %d, want 0", got)
	}
}

func TestTestExecutionModel_DiffBodyLines(t *testing.T) {
	m := newTestExecutionModel()
	m.height = 18 // diffMaxLines 6

	got := m.diffBodyLines(strings.Repeat("x\n", 5)+"x", 20)
	if len(got) != 6 || got[5] != "x" {
		t.Fatalf("6 lines = %q, want all 6 without an ellipsis", got)
	}

	got = m.diffBodyLines(strings.Repeat("x\n", 6)+"x", 20)
	if len(got) != 6 || got[4] != "x" || got[5] != "…" {
		t.Fatalf("7 lines = %q, want 5 and an ellipsis", got)
	}
}

func TestTestExecutionModel_DiffHeader(t *testing.T) {
	m := newTestExecutionModel()
	if got := m.diffHeader(); got != "Diff" {
		t.Fatalf("diffHeader without path = %q", got)
	}

	m.selectedDiffPath = "a.go"
	if got := m.diffHeader(); got != "Diff • a.go" {
		t.Fatalf("diffHeader = %q", got)
	}
}

func TestNewTestExecutionModel_Defaults(t *testing.T) {
	m := newTestExecutionModel()

	if m.progressBar.Width != 40 {
		t.Errorf("progress width = %d, want 40", m.progressBar.Width)
	}

	if m.resultsList.Width() != 80 || m.resultsList.Height() != 20 {
		t.Errorf("results list = %dx%d, want 80x20", m.resultsList.Width(), m.resultsList.Height())
	}

	if m.resultsList.ShowPagination() || !m.resultsList.ShowFilter() || m.resultsList.ShowHelp() ||
		m.resultsList.ShowTitle() || m.resultsList.ShowStatusBar() {
		t.Errorf("results list chrome: pagination %v filter %v help %v title %v status %v; want only the filter",
			m.resultsList.ShowPagination(), m.resultsList.ShowFilter(), m.resultsList.ShowHelp(),
			m.resultsList.ShowTitle(), m.resultsList.ShowStatusBar())
	}

	if m.lastSelected != -1 {
		t.Errorf("lastSelected = %d, want -1", m.lastSelected)
	}

	if startTickInterval != 100*time.Millisecond || animationTickInterval != 150*time.Millisecond {
		t.Errorf("tick intervals = %v, %v; want 100ms, 150ms", startTickInterval, animationTickInterval)
	}
}

func TestTestExecutionModel_HeaderAndCountMessages(t *testing.T) {
	m := newTestExecutionModel().handleHeaderMsg(mutationScoreMsg{score: domain.MutationScore{Killed: 2}})
	if !m.mutationScoreSet || m.mutationScore.Killed != 2 {
		t.Fatalf("score msg: set %v score %+v", m.mutationScoreSet, m.mutationScore)
	}

	m.progressPercent = 0.5
	m = m.handleConcurrency(concurrencyMsg{threads: 2, shardIndex: 1, shards: 3})
	if m.progressPercent != 0 || m.totalShards != 3 || !m.counting || !m.rendered {
		t.Fatalf("concurrency: percent %v shards %d counting %v rendered %v", m.progressPercent, m.totalShards, m.counting, m.rendered)
	}

	for _, tt := range []struct {
		count    int
		finished bool
		percent  float64
	}{{-1, false, 0}, {0, true, 1}, {1, false, 0}} {
		m := newTestExecutionModel()
		m.progressPercent = 0.5
		m.counting = true

		m = m.handleUpcoming(upcomingMsg{count: tt.count})
		if m.testingFinished != tt.finished || m.progressPercent != tt.percent || m.counting || !m.rendered {
			t.Errorf("upcoming %d: finished %v percent %v counting %v rendered %v; want finished %v percent %v",
				tt.count, m.testingFinished, m.progressPercent, m.counting, m.rendered, tt.finished, tt.percent)
		}
	}
}

func TestTestExecutionModel_CountStatus(t *testing.T) {
	m := newTestExecutionModel()
	m.results = []testResult{{status: "error"}, {status: "killed"}, {status: "survived"}, {status: "killed"}}

	if got := m.countStatus("killed"); got != 2 {
		t.Fatalf("countStatus(killed) = %d, want 2", got)
	}
}

func TestTestExecutionModel_CompletionFreesOnlyItsThread(t *testing.T) {
	m := newTestExecutionModel()
	m.totalMutations = 5
	m = m.handleStartMutation(startMutationMsg{id: "aaaa", thread: 0, displayPath: "a.go"})
	m = m.handleStartMutation(startMutationMsg{id: "cccc", thread: 1, displayPath: "c.go"})

	m = m.handleCompletedMutation(completedMutationMsg{id: "bbbb", displayPath: "b.go", status: "killed"})
	if m.threadFiles[0] != "a.go" || m.threadFiles[1] != "c.go" {
		t.Fatalf("threads after an unrelated completion = %v, want both busy", m.threadFiles)
	}
}

func TestTestExecutionModel_CompletionProgress(t *testing.T) {
	for _, tt := range []struct {
		name        string
		total, done int
		finished    bool
		percent     float64
	}{
		{"partway", 4, 1, false, 0.5},
		{"done", 2, 1, true, 1},
		{"past the total", 1, 1, false, 2},
		{"no total", 0, 0, true, 1},
		{"negative total", -1, 0, false, 0},
	} {
		m := newTestExecutionModel()
		m.totalMutations = tt.total
		m.completedCount = tt.done

		m = m.handleCompletedMutation(completedMutationMsg{id: "abcd", displayPath: "a.go", status: "killed"})
		if m.testingFinished != tt.finished || m.progressPercent != tt.percent {
			t.Errorf("%s: finished %v percent %v; want %v %v", tt.name, m.testingFinished, m.progressPercent, tt.finished, tt.percent)
		}
	}
}

// diffModel returns a finished model with two results that have different
// diffs, the first one selected and its diff open.
func diffModel() testExecutionModel {
	m := newTestExecutionModel()
	m.testingFinished = true
	m.resultsList.SetItems([]list.Item{
		testResult{id: "1", file: "a.go", status: "survived", diff: "-a\n+b"},
		testResult{id: "2", file: "b.go", status: "survived", diff: "-c\n+d"},
	})
	m.resultsList.Select(0)
	m.lastSelected = 0
	m.toggleSelectedDiff()

	return m
}

func TestTestExecutionModel_KeysToggleDiffOnlyOnEnterAndSpace(t *testing.T) {
	m := diffModel()

	m, _ = m.handleKeyMsg(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")})
	if m.showDiff {
		t.Fatalf("space should hide the open diff")
	}

	m = diffModel()

	m, _ = m.handleKeyMsg(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{0x1f}})
	if !m.showDiff {
		t.Fatalf("a control character should not toggle the diff")
	}

	m = diffModel()
	m.animOffset, m.delegate.offset = 4, 4

	m, _ = m.handleKeyMsg(tea.KeyMsg{Type: tea.KeyDown})
	if m.resultsList.Index() != 1 || m.lastSelected != 1 || m.showDiff || m.selectedDiff != "" || m.selectedDiffPath != "" ||
		m.animOffset != 0 || m.delegate.offset != 0 {
		t.Fatalf("down: index %d last %d diff %v %q %q offsets %d %d; want the next row, diff closed, offsets reset",
			m.resultsList.Index(), m.lastSelected, m.showDiff, m.selectedDiff, m.selectedDiffPath, m.animOffset, m.delegate.offset)
	}

	m = diffModel()
	m.resultsList.Select(1)
	m.lastSelected = 1

	m, _ = m.handleKeyMsg(tea.KeyMsg{Type: tea.KeyUp})
	if m.resultsList.Index() != 0 || m.lastSelected != 0 || m.showDiff {
		t.Fatalf("up: index %d last %d diff %v; want row 0, diff closed", m.resultsList.Index(), m.lastSelected, m.showDiff)
	}
}

func TestTestExecutionModel_MouseResetsAndToggles(t *testing.T) {
	release := tea.MouseMsg(tea.MouseEvent{Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})

	m := diffModel()
	m.lastSelected = 1
	m.animOffset, m.delegate.offset = 4, 4

	m, _ = m.handleMouseMsg(tea.MouseMsg(tea.MouseEvent{Button: tea.MouseButtonNone, Action: tea.MouseActionMotion}))
	if m.lastSelected != 0 || m.showDiff || m.animOffset != 0 || m.delegate.offset != 0 {
		t.Fatalf("selection change: last %d diff %v offsets %d %d; want 0, closed, reset", m.lastSelected, m.showDiff, m.animOffset, m.delegate.offset)
	}

	m = diffModel()
	m.lastSelected = -1

	m, _ = m.handleMouseMsg(tea.MouseMsg(tea.MouseEvent{Button: tea.MouseButtonNone, Action: tea.MouseActionMotion}))
	if m.lastSelected != 0 || m.showDiff {
		t.Fatalf("selection from none: last %d diff %v; want 0, closed", m.lastSelected, m.showDiff)
	}

	for _, ev := range []tea.MouseEvent{
		{Button: tea.MouseButtonNone, Action: tea.MouseActionRelease},
		{Button: tea.MouseButtonMiddle, Action: tea.MouseActionRelease},
		{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress},
		{Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion},
	} {
		m := diffModel()
		if m, _ = m.handleMouseMsg(tea.MouseMsg(ev)); !m.showDiff {
			t.Errorf("%+v closed the diff; only a left release toggles", ev)
		}
	}

	m = diffModel()
	m.resultsList.SetFilterText("a")
	m.resultsList.SetFilterState(list.FilterApplied)

	if m, _ = m.handleMouseMsg(release); m.showDiff {
		t.Fatalf("left release with a filter applied should toggle the diff")
	}

	m = diffModel()
	m.resultsList.SetFilterState(list.Filtering)

	if m, _ = m.handleMouseMsg(release); !m.showDiff {
		t.Fatalf("left release while typing a filter should not toggle the diff")
	}
}

func TestTestExecutionModel_ToggleSelectedDiffSwitchesDiffs(t *testing.T) {
	m := diffModel()
	m.resultsList.Select(1)
	m.toggleSelectedDiff()

	if !m.showDiff || m.selectedDiff != "-c\n+d" || m.selectedDiffPath != "b.go" {
		t.Fatalf("open diff of another row: %v %q %q; want b.go's diff", m.showDiff, m.selectedDiff, m.selectedDiffPath)
	}

	m.resultsList.Select(0)
	m.toggleSelectedDiff()

	if !m.showDiff || m.selectedDiff != "-a\n+b" {
		t.Fatalf("back to a.go: %v %q; want a.go's diff", m.showDiff, m.selectedDiff)
	}

	m.showDiff = false
	m.toggleSelectedDiff()

	if !m.showDiff {
		t.Fatalf("a closed diff of the same row should open")
	}
}

func TestTestExecutionModel_TickAnimatesUnlessFiltering(t *testing.T) {
	for _, tt := range []struct {
		state   list.FilterState
		advance bool
	}{{list.Unfiltered, true}, {list.Filtering, false}, {list.FilterApplied, true}} {
		m := newTestExecutionModel()
		m.testingFinished = true
		m.resultsList.SetItems([]list.Item{testResult{id: "1", file: "a.go"}})
		m.resultsList.SetFilterText("a")
		m.resultsList.SetFilterState(tt.state)

		m, cmd := m.handleTickMsg(tickMsg(time.Now()))
		if cmd == nil {
			t.Fatalf("tick returned no next tick")
		}

		if advanced := m.animOffset == 1 && m.delegate.offset == 1; advanced != tt.advance {
			t.Errorf("state %v: offsets %d %d, want advanced %v", tt.state, m.animOffset, m.delegate.offset, tt.advance)
		}
	}
}

func TestTestResultDelegate_RenderSelectionScrolls(t *testing.T) {
	m := finishedModel(70, 22)
	delegate := testResultDelegate{offset: 9}

	var rows []string

	for i, item := range m.resultsList.Items() {
		var b strings.Builder
		delegate.Render(&b, m.resultsList, i, item)
		rows = append(rows, b.String())
	}

	assertGolden(t, "delegate_rows", strings.Join(rows, "\n"))
}
