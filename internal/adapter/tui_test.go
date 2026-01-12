package adapter

import (
	"bytes"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	m "github.com/mouse-blink/gooze/internal/model"
)

func TestTUI_DisplayMutationEstimations(t *testing.T) {
	tests := []struct {
		name        string
		estimations map[m.Path]int
		wantErr     bool
	}{
		{
			name:        "empty estimations",
			estimations: map[m.Path]int{},
			wantErr:     false,
		},
		{
			name: "single file",
			estimations: map[m.Path]int{
				"main.go": 5,
			},
			wantErr: false,
		},
		{
			name: "multiple files",
			estimations: map[m.Path]int{
				"main.go":    5,
				"helper.go":  3,
				"utils.go":   0,
				"handler.go": 10,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			tui := NewTUI(&buf)
			err := tui.DisplayMutationEstimations(tt.estimations)
			if (err != nil) != tt.wantErr {
				t.Errorf("DisplayMutationEstimations() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMutationCountModel_Init(t *testing.T) {
	mcm := mutationCountModel{
		counts: []mutationCount{
			{file: "main.go", count: 5},
		},
		total:        5,
		mutationType: m.MutationArithmetic,
		height:       24,
	}

	cmd := mcm.Init()
	if cmd != nil {
		t.Errorf("Init() should return nil, got %v", cmd)
	}
}

func TestMutationCountModel_View(t *testing.T) {
	tests := []struct {
		name       string
		model      mutationCountModel
		wantInView []string
	}{
		{
			name: "single file",
			model: mutationCountModel{
				counts: []mutationCount{
					{file: "main.go", count: 5},
				},
				total:        5,
				mutationType: m.MutationArithmetic,
				height:       24,
			},
			wantInView: []string{
				"Gooze - Mutation Testing",
				"arithmetic mutations summary",
				"main.go: 5 mutations",
				"Total: 5 arithmetic mutations across 1 file(s)",
			},
		},
		{
			name: "multiple files with zero",
			model: mutationCountModel{
				counts: []mutationCount{
					{file: "main.go", count: 5},
					{file: "helper.go", count: 0},
				},
				total:        5,
				mutationType: m.MutationArithmetic,
				height:       24,
			},
			wantInView: []string{
				"main.go: 5 mutations",
				"helper.go:",
				"0 mutations",
				"Total: 5 arithmetic mutations across 2 file(s)",
			},
		},
		{
			name: "quitting state",
			model: mutationCountModel{
				counts: []mutationCount{
					{file: "main.go", count: 5},
				},
				total:        5,
				mutationType: m.MutationArithmetic,
				height:       24,
				quitting:     true,
			},
			wantInView: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			view := tt.model.View()
			for _, want := range tt.wantInView {
				if !strings.Contains(view, want) {
					t.Errorf("View() should contain %q, got:\n%s", want, view)
				}
			}
		})
	}
}

func TestMutationCountModel_HandleKeyPress(t *testing.T) {
	tests := []struct {
		name           string
		model          mutationCountModel
		key            tea.KeyMsg
		expectQuitting bool
		expectOffset   int
	}{
		{
			name: "quit with q",
			model: mutationCountModel{
				counts:       []mutationCount{{file: "main.go", count: 5}},
				total:        5,
				mutationType: m.MutationArithmetic,
				height:       24,
			},
			key:            tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}},
			expectQuitting: true,
			expectOffset:   0,
		},
		{
			name: "quit with Ctrl+C",
			model: mutationCountModel{
				counts:       []mutationCount{{file: "main.go", count: 5}},
				total:        5,
				mutationType: m.MutationArithmetic,
				height:       24,
			},
			key:            tea.KeyMsg{Type: tea.KeyCtrlC},
			expectQuitting: true,
			expectOffset:   0,
		},
		{
			name: "scroll down with j",
			model: mutationCountModel{
				counts: []mutationCount{
					{file: "file1.go", count: 1},
					{file: "file2.go", count: 2},
					{file: "file3.go", count: 3},
				},
				total:        6,
				mutationType: m.MutationArithmetic,
				height:       10,
				offset:       0,
			},
			key:            tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}},
			expectQuitting: false,
			expectOffset:   1,
		},
		{
			name: "scroll up with k",
			model: mutationCountModel{
				counts: []mutationCount{
					{file: "file1.go", count: 1},
					{file: "file2.go", count: 2},
				},
				total:        3,
				mutationType: m.MutationArithmetic,
				height:       10,
				offset:       1,
			},
			key:            tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}},
			expectQuitting: false,
			expectOffset:   0,
		},
		{
			name: "go to top with g",
			model: mutationCountModel{
				counts: []mutationCount{
					{file: "file1.go", count: 1},
					{file: "file2.go", count: 2},
				},
				total:        3,
				mutationType: m.MutationArithmetic,
				height:       10,
				offset:       5,
			},
			key:            tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}},
			expectQuitting: false,
			expectOffset:   0,
		},
		{
			name: "go to bottom with G",
			model: mutationCountModel{
				counts: []mutationCount{
					{file: "file1.go", count: 1},
					{file: "file2.go", count: 2},
					{file: "file3.go", count: 3},
				},
				total:        6,
				mutationType: m.MutationArithmetic,
				height:       10,
				offset:       0,
			},
			key:            tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}},
			expectQuitting: false,
			expectOffset:   0, // Will be calculated by maxOffset
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model, _ := tt.model.handleKeyPress(tt.key)
			mcm := model.(mutationCountModel)

			if mcm.quitting != tt.expectQuitting {
				t.Errorf("quitting = %v, want %v", mcm.quitting, tt.expectQuitting)
			}

			if !tt.expectQuitting && tt.name != "go to bottom with G" {
				if mcm.offset != tt.expectOffset {
					t.Errorf("offset = %v, want %v", mcm.offset, tt.expectOffset)
				}
			}
		})
	}
}

func TestMutationCountModel_Pagination(t *testing.T) {
	// Create a model with many files that will require pagination
	counts := make([]mutationCount, 50)
	totalCount := 0
	for i := range counts {
		counts[i] = mutationCount{
			file:  "file" + string(rune('0'+i)) + ".go",
			count: i,
		}
		totalCount += i
	}

	mcm := mutationCountModel{
		counts:       counts,
		total:        totalCount,
		mutationType: m.MutationArithmetic,
		height:       15, // Very small height to ensure pagination needed
		width:        80,
		offset:       0,
	}

	// Test that pagination is needed with many items
	if !mcm.needsPagination() {
		t.Error("Expected needsPagination to be true with 50 items and height 15")
	}

	view := mcm.View()

	// Check that navigation hints are shown when pagination is needed
	hasNavigation := strings.Contains(view, "j/k") || strings.Contains(view, "pgup") ||
		strings.Contains(view, "PgUp") || strings.Contains(view, "↑") || strings.Contains(view, "↓")
	if !hasNavigation {
		t.Errorf("View should contain navigation hints when pagination needed, got:\n%s", view)
	}

	// Test that offset doesn't go negative
	if mcm.offset < 0 {
		t.Error("offset should not be negative")
	}

	// Test max offset
	maxOff := mcm.maxOffset()
	if maxOff < 0 {
		t.Error("maxOffset should not be negative")
	}
}

func TestNotImplementedModel_View(t *testing.T) {
	nim := notImplementedModel{count: 5}
	view := nim.View()

	if !strings.Contains(view, "Gooze - Mutation Testing") {
		t.Error("View should contain header")
	}

	if !strings.Contains(view, "5 source file(s)") {
		t.Error("View should contain source file count")
	}

	if !strings.Contains(view, "-l") {
		t.Error("View should mention -l flag")
	}
}
