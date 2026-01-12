package adapter

import (
	"bytes"
	"testing"

	m "github.com/mouse-blink/gooze/internal/model"
	"github.com/spf13/cobra"
)

func TestSimpleUI_DisplayMutationEstimations(t *testing.T) {
	tests := []struct {
		name        string
		estimations map[m.Path]int
		want        string
	}{
		{
			name:        "empty estimations",
			estimations: map[m.Path]int{},
			want:        "\nTotal: 0 arithmetic mutations across 0 files\n",
		},
		{
			name: "single file with mutations",
			estimations: map[m.Path]int{
				m.Path("main.go"): 4,
			},
			want: "main.go: 4 arithmetic mutations\n\nTotal: 4 arithmetic mutations across 1 files\n",
		},
		{
			name: "multiple files with mutations",
			estimations: map[m.Path]int{
				m.Path("main.go"):   4,
				m.Path("helper.go"): 8,
				m.Path("types.go"):  0,
			},
			want: "helper.go: 8 arithmetic mutations\nmain.go: 4 arithmetic mutations\ntypes.go: 0 arithmetic mutations\n\nTotal: 12 arithmetic mutations across 3 files\n",
		},
		{
			name: "files with zero mutations",
			estimations: map[m.Path]int{
				m.Path("empty.go"): 0,
			},
			want: "empty.go: 0 arithmetic mutations\n\nTotal: 0 arithmetic mutations across 1 files\n",
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
			if got != tt.want {
				t.Errorf("DisplayMutationEstimations() output = %q, want %q", got, tt.want)
			}
		})
	}
}
