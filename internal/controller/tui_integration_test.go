package controller

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	m "github.com/mouse-blink/gooze/internal/model"
)

// TestEstimateModelIntegration tests the full lifecycle of estimateModel with Bubble Tea
func TestEstimateModelIntegration(t *testing.T) {
	model := newEstimateModel()

	// Init should return a tick command
	cmd := model.Init()
	if cmd == nil {
		t.Fatalf("Init() returned nil")
	}

	// Execute init command to get tick message
	msg := cmd()
	if _, ok := msg.(tickMsg); !ok {
		t.Fatalf("Init() cmd did not return tickMsg")
	}

	// Send window size
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model = updated.(estimateModel)

	// Send estimation data
	estMsg := estimationMsg{
		total:     5,
		paths:     2,
		fileStats: map[string]int{"a.go": 3, "b.go": 2},
	}
	updated, _ = model.Update(estMsg)
	model = updated.(estimateModel)

	// View should now show the estimate
	view := model.View()
	if !strings.Contains(view, "Gooze Mutation Estimate") {
		t.Fatalf("View missing title")
	}
	if !strings.Contains(view, "5") {
		t.Fatalf("View missing total count")
	}

	// Send tick to trigger animation
	updated, cmd = model.Update(tickMsg(time.Now()))
	model = updated.(estimateModel)
	if cmd == nil {
		t.Fatalf("Update tick did not return cmd")
	}

	// Key navigation
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(estimateModel)

	// Quit
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatalf("Quit key did not return tea.Quit")
	}
}

// TestTestExecutionModelIntegration tests the full lifecycle of testExecutionModel
func TestTestExecutionModelIntegration(t *testing.T) {
	model := newTestExecutionModel()

	// Init should return a tick command
	cmd := model.Init()
	if cmd == nil {
		t.Fatalf("Init() returned nil")
	}

	// Execute init command
	msg := cmd()
	if _, ok := msg.(tickMsg); !ok {
		t.Fatalf("Init() cmd did not return tickMsg")
	}

	// View before rendering
	view := model.View()
	if !strings.Contains(view, "Initializing") {
		t.Fatalf("View before render should show initializing")
	}

	// Send window size
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model = updated.(testExecutionModel)

	// Send concurrency info
	updated, _ = model.Update(concurrencyMsg{threads: 2, shardIndex: 1, shards: 2})
	model = updated.(testExecutionModel)

	// Send upcoming count
	updated, _ = model.Update(upcomingMsg{count: 10})
	model = updated.(testExecutionModel)

	// Start a mutation
	startMsg := startMutationMsg{id: 1, thread: 0, kind: m.MutationArithmetic, path: "test.go"}
	updated, _ = model.Update(startMsg)
	model = updated.(testExecutionModel)

	// View should show progress
	view = model.View()
	if !strings.Contains(view, "Gooze Mutation Testing") {
		t.Fatalf("View missing title")
	}

	// Complete the mutation
	completeMsg := completedMutationMsg{id: 1, kind: m.MutationArithmetic, status: "killed"}
	updated, _ = model.Update(completeMsg)
	model = updated.(testExecutionModel)

	// Send tick
	updated, cmd = model.Update(tickMsg(time.Now()))
	model = updated.(testExecutionModel)
	if cmd == nil {
		t.Fatalf("Tick did not return cmd")
	}

	// Verify progress
	if model.completedCount != 1 {
		t.Fatalf("completedCount = %d, want 1", model.completedCount)
	}

	// Complete remaining mutations to finish
	for i := 2; i <= 10; i++ {
		completeMsg := completedMutationMsg{id: i, kind: m.MutationBoolean, status: "survived"}
		updated, _ = model.Update(completeMsg)
		model = updated.(testExecutionModel)
	}

	// Should be finished
	if !model.testingFinished {
		t.Fatalf("testingFinished = false, want true")
	}

	// View should show results
	view = model.View()
	if !strings.Contains(view, "Gooze Test Results") {
		t.Fatalf("View missing results title")
	}

	// Navigate results
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(testExecutionModel)

	// Quit
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatalf("Quit key did not return tea.Quit")
	}
}

// TestEstimateModelAnimationCoverage tests animation edge cases
func TestEstimateModelAnimationCoverage(t *testing.T) {
	// Test animateScroll with empty string
	if got := animateScroll("", 10, 0); got != "" {
		t.Fatalf("animateScroll empty string = %q", got)
	}

	// Test with width larger than text
	if got := animateScroll("short", 20, 5); got != "short" {
		t.Fatalf("animateScroll short = %q", got)
	}

	// Test scrolling behavior
	text := "verylongtext"
	got1 := animateScroll(text, 5, 10)
	got2 := animateScroll(text, 5, 15)
	if got1 == got2 {
		t.Fatalf("animateScroll should change with offset")
	}

	// Test truncateToWidth edge cases
	if got := truncateToWidth("", 10); got != "" {
		t.Fatalf("truncateToWidth empty = %q", got)
	}

	if got := truncateToWidth("test", 2); len([]rune(got)) != 2 {
		t.Fatalf("truncateToWidth length = %d, want 2", len([]rune(got)))
	}
}

// TestTestExecutionModelAnimationCoverage tests test execution animation helpers
func TestTestExecutionModelAnimationCoverage(t *testing.T) {
	// Test animateScrollFile with empty string
	if got := animateScrollFile("", 10, 0); got != "" {
		t.Fatalf("animateScrollFile empty string = %q", got)
	}

	// Test with width larger than text
	if got := animateScrollFile("short", 20, 5); got != "short" {
		t.Fatalf("animateScrollFile short = %q", got)
	}

	// Test scrolling behavior
	text := "verylongfilepath.go"
	got1 := animateScrollFile(text, 5, 10)
	got2 := animateScrollFile(text, 5, 15)
	if got1 == got2 {
		t.Fatalf("animateScrollFile should change with offset")
	}

	// Test truncateFile edge cases
	if got := truncateFile("", 10); got != "" {
		t.Fatalf("truncateFile empty = %q", got)
	}

	if got := truncateFile("test", 2); len([]rune(got)) != 2 {
		t.Fatalf("truncateFile length = %d, want 2", len([]rune(got)))
	}
}

// TestRenderThreadBoxEdgeCases covers remaining renderThreadBox branches
func TestRenderThreadBoxEdgeCases(t *testing.T) {
	m := newTestExecutionModel()
	m.width = 100
	m.height = 30

	// Test with very narrow width
	m.width = 10
	box := m.renderThreadBox("6")
	if box == "" {
		t.Fatalf("renderThreadBox should not be empty")
	}

	// Test with file but no mutation ID
	m.width = 100
	m.threads = 1
	m.threadFiles = map[int]string{0: "path/to/file.go"}
	m.threadMutationIDs = map[int]string{}
	box = m.renderThreadBox("6")
	if !strings.Contains(box, "file.go") {
		t.Fatalf("renderThreadBox missing filename")
	}

	// Test with both file and mutation ID
	m.threadMutationIDs = map[int]string{0: "123"}
	box = m.renderThreadBox("6")
	if !strings.Contains(box, "ID:") {
		t.Fatalf("renderThreadBox missing ID label")
	}
}

// TestRenderResultsBoxEdgeCases covers remaining renderResultsBox branches
func TestRenderResultsBoxEdgeCases(t *testing.T) {
	m := newTestExecutionModel()
	m.width = 100
	m.height = 30

	// Test with very small height
	m.height = 5
	box := m.renderResultsBox("6")
	if !strings.Contains(box, "ID") {
		t.Fatalf("renderResultsBox missing headers")
	}

	// Test with normal size
	m.height = 30
	m.width = 80
	box = m.renderResultsBox("6")
	if !strings.Contains(box, "Status") {
		t.Fatalf("renderResultsBox missing Status header")
	}
}

// TestEstimateModelUpdateEdgeCases covers remaining Update branches
func TestEstimateModelUpdateEdgeCases(t *testing.T) {
	m := newEstimateModel()

	// Test ctrl+c quit
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatalf("Ctrl+C should return quit cmd")
	}
	_ = updated

	// Test with rendered but filtering
	m.rendered = true
	m.fileList.SetItems([]list.Item{fileItem{path: "a.go", count: 1}})
	m.fileList.SetFilteringEnabled(true)
	_, _ = m.fileList.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})

	updated, cmd = m.Update(tickMsg(time.Now()))
	// When filtering, tick should not increment animation
	_ = updated
	_ = cmd
}
