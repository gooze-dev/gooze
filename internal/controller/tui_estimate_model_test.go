package controller

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func TestAnimateScroll_Edges(t *testing.T) {
	if got := animateScroll("hello", 0, 0); got != "" {
		t.Fatalf("animateScroll width 0 = %q, want empty", got)
	}

	if got := animateScroll("hi", 5, 0); got != "hi" {
		t.Fatalf("animateScroll short text = %q, want hi", got)
	}

	if got := animateScroll("abcdef", 3, 0); got != "ab…" {
		t.Fatalf("animateScroll pause = %q, want ab…", got)
	}

	got := animateScroll("abcdef", 3, 10)
	if got == "ab…" || len([]rune(got)) != 3 {
		t.Fatalf("animateScroll scrolled = %q, want len 3 and not truncated", got)
	}
}

func TestTruncateToWidth(t *testing.T) {
	if got := truncateToWidth("hello", 0); got != "" {
		t.Fatalf("truncateToWidth width 0 = %q, want empty", got)
	}

	if got := truncateToWidth("hello", 10); got != "hello" {
		t.Fatalf("truncateToWidth no truncation = %q", got)
	}

	if got := truncateToWidth("hello", 1); got != "…" {
		t.Fatalf("truncateToWidth width 1 = %q, want ellipsis", got)
	}

	if got := truncateToWidth("hello", 2); got != "h…" {
		t.Fatalf("truncateToWidth width 2 = %q, want h…", got)
	}
}

func TestEstimateModel_HandleEstimationMsgAndView(t *testing.T) {
	m := newEstimateModel()
	if got := m.View(); got != "Loading mutation list…\n" {
		t.Fatalf("View() before render = %q", got)
	}

	msg := estimationMsg{
		total: 3,
		paths: 2,
		fileStats: map[string]int{
			"b.go": 1,
			"a.go": 2,
		},
	}

	m = m.handleEstimationMsg(msg)
	if !m.rendered || m.total != 3 || m.totalFiles != 2 {
		t.Fatalf("handleEstimationMsg did not set totals or rendered")
	}

	if m.lastSelected != 0 {
		t.Fatalf("lastSelected = %d, want 0", m.lastSelected)
	}

	m.width = 80
	m.height = 25
	view := m.View()
	if !strings.Contains(view, "Gooze Mutation Estimate") {
		t.Fatalf("View() missing title\n%s", view)
	}

	if cmd := m.Init(); cmd == nil {
		t.Fatalf("Init() returned nil cmd")
	}

	table := m.renderTable()
	if !strings.Contains(table, "Count") || !strings.Contains(table, "File Path") {
		t.Fatalf("renderTable missing headers\n%s", table)
	}

	// force small height to hit min list height branch
	m.height = 0
	m.width = 20
	_ = m.renderTable()
}

func TestEstimateModel_UpdateBranches(t *testing.T) {
	m := newEstimateModel()
	m.rendered = true
	m.fileList.SetItems([]list.Item{fileItem{path: "a", count: 1}})
	_, _ = m.fileList.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})

	model, cmd := m.Update(tickMsg(time.Now()))
	if cmd == nil {
		t.Fatalf("expected tick cmd")
	}
	updated := model.(estimateModel)
	if updated.animOffset != 1 {
		t.Fatalf("animOffset = %d, want 1", updated.animOffset)
	}

	model, _ = updated.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	updated = model.(estimateModel)
	if updated.width != 100 || updated.height != 40 {
		t.Fatalf("window size not applied")
	}

	model, cmd = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatalf("expected quit cmd")
	}
	_ = model

	model, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	updated = model.(estimateModel)
	if updated.lastSelected == -1 {
		t.Fatalf("expected selection to be tracked")
	}

	// Set filtering state and test tick returns nil
	updated.fileList.SetFilteringEnabled(true)
	_, _ = updated.fileList.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	model, cmd = updated.Update(tickMsg(time.Now()))
	_ = model

	updated.rendered = false
	model, _ = updated.Update(estimationMsg{total: 1, paths: 1, fileStats: map[string]int{"a": 1}})
	if !model.(estimateModel).rendered {
		t.Fatalf("expected rendered after estimationMsg")
	}

	model, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	_ = model
}

func TestEstimateDelegate_Render(t *testing.T) {
	delegate := estimateDelegate{offset: 0}
	items := []list.Item{fileItem{path: "path/to/file.go", count: 2}}
	m := list.New(items, delegate, 20, 5)

	var buf bytes.Buffer
	delegate.Render(&buf, m, 0, items[0])
	if !strings.Contains(buf.String(), "path") {
		t.Fatalf("render output missing path")
	}

	buf.Reset()
	delegate.Render(&buf, m, 1, items[0])
	if buf.Len() == 0 {
		t.Fatalf("render output empty")
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
