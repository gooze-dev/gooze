package controller

import (
	"bytes"
	"testing"

	m "github.com/mouse-blink/gooze/internal/model"
	"github.com/spf13/cobra"
)

func TestSimpleUI_DisplayMutationEstimations(t *testing.T) {
	tests := []struct {
		name         string
		estimations  map[m.Path]MutationEstimation
		wantContains []string
	}{
		{
			name:         "empty estimations",
			estimations:  map[m.Path]MutationEstimation{},
			wantContains: []string{"0 mutations", "0 files"},
		},
		{
			name: "single file with mutations",
			estimations: map[m.Path]MutationEstimation{
				m.Path("main.go"): {Arithmetic: 4, Boolean: 2},
			},
			wantContains: []string{"main.go", "4 arithmetic", "2 boolean", "Total across"},
		},
		{
			name: "multiple files with mutations",
			estimations: map[m.Path]MutationEstimation{
				m.Path("main.go"):   {Arithmetic: 4, Boolean: 0},
				m.Path("helper.go"): {Arithmetic: 8, Boolean: 3},
				m.Path("types.go"):  {Arithmetic: 0, Boolean: 1},
			},
			wantContains: []string{"helper.go", "main.go", "types.go", "12 arithmetic", "4 boolean"},
		},
		{
			name: "files with zero mutations",
			estimations: map[m.Path]MutationEstimation{
				m.Path("empty.go"): {Arithmetic: 0, Boolean: 0},
			},
			wantContains: []string{"empty.go", "0 arithmetic", "0 boolean"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a cobra command with a buffer to capture output
			var buf bytes.Buffer
			cmd := &cobra.Command{}
			cmd.SetOut(&buf)

			// Create UI and display estimations
			ui := NewSimpleUI(cmd)
			err := ui.DisplayMutationEstimations(tt.estimations)

			if err != nil {
				t.Errorf("DisplayMutationEstimations() error = %v", err)
				return
			}

			got := buf.String()
			for _, want := range tt.wantContains {
				if !contains(got, want) {
					t.Errorf("DisplayMutationEstimations() output missing %q, got: %s", want, got)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestSimpleUI_DisplayMutationResults(t *testing.T) {
	tests := []struct {
		name         string
		sources      []m.Source
		fileResults  map[m.Path]interface{}
		wantContains []string
	}{
		{
			name:         "empty sources",
			sources:      []m.Source{},
			fileResults:  map[m.Path]interface{}{},
			wantContains: []string{"Mutation Testing Results", "Summary", "Total: 0"},
		},
		{
			name: "single file with killed mutations",
			sources: []m.Source{
				{Origin: m.Path("main.go")},
			},
			fileResults: map[m.Path]interface{}{
				m.Path("main.go"): m.FileResult{
					Reports: []m.Report{
						{MutationID: "ARITH_1", Killed: true},
						{MutationID: "ARITH_2", Killed: true},
					},
				},
			},
			wantContains: []string{"main.go", "2 mutations", "ARITH_1", "killed", "100.0%"},
		},
		{
			name: "single file with survived mutations",
			sources: []m.Source{
				{Origin: m.Path("main.go")},
			},
			fileResults: map[m.Path]interface{}{
				m.Path("main.go"): m.FileResult{
					Reports: []m.Report{
						{MutationID: "ARITH_1", Killed: false},
					},
				},
			},
			wantContains: []string{"main.go", "1 mutations", "ARITH_1", "survived", "0.0%"},
		},
		{
			name: "multiple files with mixed results",
			sources: []m.Source{
				{Origin: m.Path("main.go")},
				{Origin: m.Path("helper.go")},
			},
			fileResults: map[m.Path]interface{}{
				m.Path("main.go"): m.FileResult{
					Reports: []m.Report{
						{MutationID: "ARITH_1", Killed: true},
					},
				},
				m.Path("helper.go"): m.FileResult{
					Reports: []m.Report{
						{MutationID: "BOOL_1", Killed: false},
					},
				},
			},
			wantContains: []string{"main.go", "helper.go", "Killed: 1", "Survived: 1", "50.0%"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			cmd := &cobra.Command{}
			cmd.SetOut(&buf)

			ui := NewSimpleUI(cmd)
			err := ui.DisplayMutationResults(tt.sources, tt.fileResults)

			if err != nil {
				t.Errorf("DisplayMutationResults() error = %v", err)
				return
			}

			got := buf.String()
			t.Logf("Output:\n%s", got)

			for _, want := range tt.wantContains {
				if !contains(got, want) {
					t.Errorf("DisplayMutationResults() output missing %q", want)
				}
			}
		})
	}
}

func TestExtractReportsFromResult(t *testing.T) {
	t.Run("extracts reports from FileResult value", func(t *testing.T) {
		fr := m.FileResult{
			Reports: []m.Report{
				{MutationID: "TEST_1", Killed: true},
				{MutationID: "TEST_2", Killed: false},
			},
		}

		var i interface{} = fr
		reports := extractReportsFromResult(i)

		t.Logf("Input type: %T", i)
		t.Logf("Reports: %+v", reports)

		if len(reports) != 2 {
			t.Errorf("expected 2 reports, got %d", len(reports))
		}
	})

	t.Run("extracts reports from FileResult pointer", func(t *testing.T) {
		fr := &m.FileResult{
			Reports: []m.Report{
				{MutationID: "TEST_1", Killed: true},
			},
		}

		var i interface{} = fr
		reports := extractReportsFromResult(i)

		t.Logf("Input type: %T", i)
		t.Logf("Reports: %+v", reports)

		if len(reports) != 1 {
			t.Errorf("expected 1 report, got %d", len(reports))
		}
	})

	t.Run("returns nil for nil input", func(t *testing.T) {
		reports := extractReportsFromResult(nil)
		if reports != nil {
			t.Errorf("expected nil, got %v", reports)
		}
	})
}
