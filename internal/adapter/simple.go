package adapter

import (
	m "github.com/mouse-blink/gooze/internal/model"
	"github.com/spf13/cobra"
)

// SimpleUI implements UI using cobra Command's Println.
type SimpleUI struct {
	cmd *cobra.Command
}

// NewSimpleUI creates a new SimpleUI.
func NewSimpleUI(cmd *cobra.Command) *SimpleUI {
	return &SimpleUI{cmd: cmd}
}

// ShowNotImplemented displays a "not implemented" message.
func (p *SimpleUI) ShowNotImplemented(count int) error {
	p.cmd.Printf("Found %d source files\n", count)
	p.cmd.Println("Mutation testing not yet implemented. Use --list or -l flag to see mutation counts.")

	return nil
}

// DisplayMutationEstimations displays pre-calculated mutation estimations.
func (p *SimpleUI) DisplayMutationEstimations(estimations map[m.Path]int) error {
	if len(estimations) == 0 {
		p.cmd.Printf("\nTotal: 0 arithmetic mutations across 0 files\n")
		return nil
	}

	totalMutations := 0

	// Sort paths for consistent output
	paths := make([]m.Path, 0, len(estimations))
	for path := range estimations {
		paths = append(paths, path)
	}

	// Simple sort by string comparison
	for i := 0; i < len(paths); i++ {
		for j := i + 1; j < len(paths); j++ {
			if string(paths[i]) > string(paths[j]) {
				paths[i], paths[j] = paths[j], paths[i]
			}
		}
	}

	for _, path := range paths {
		count := estimations[path]
		p.cmd.Printf("%s: %d arithmetic mutations\n", path, count)
		totalMutations += count
	}

	p.cmd.Printf("\nTotal: %d arithmetic mutations across %d files\n", totalMutations, len(estimations))

	return nil
}
