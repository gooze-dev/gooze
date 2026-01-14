package adapter

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
