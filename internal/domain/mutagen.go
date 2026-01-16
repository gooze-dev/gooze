// Package domain contains the core mutation testing workflow and logic.
package domain

import (
	"fmt"
	"go/ast"
	"go/token"

	"github.com/mouse-blink/gooze/internal/adapter"
	"github.com/mouse-blink/gooze/internal/domain/mutagens"
	m "github.com/mouse-blink/gooze/internal/model"
)

// Mutagen defines the interface for mutation generation.
type Mutagen interface {
	GenerateMutation(source m.SourceV2, startingIndex int, mutationTypes ...m.MutationType) ([]m.MutationV2, error)
}

// mutagen handles pure mutation generation logic.
type mutagen struct {
	adapter.GoFileAdapter
	adapter.SourceFSAdapter
}

// NewMutagen creates a new Mutagen instance.
func NewMutagen(goFileAdapter adapter.GoFileAdapter, sourceFSAdapter adapter.SourceFSAdapter) Mutagen {
	return &mutagen{
		GoFileAdapter:   goFileAdapter,
		SourceFSAdapter: sourceFSAdapter,
	}
}

func (mg *mutagen) GenerateMutation(source m.SourceV2, startingIndex int, mutationTypes ...m.MutationType) ([]m.MutationV2, error) {
	if source.Origin == nil || source.Origin.Path == "" {
		return nil, fmt.Errorf("missing source origin")
	}

	if len(mutationTypes) == 0 {
		mutationTypes = []m.MutationType{m.MutationArithmetic, m.MutationBoolean}
	}

	for _, mutationType := range mutationTypes {
		if mutationType != m.MutationArithmetic && mutationType != m.MutationBoolean {
			return nil, fmt.Errorf("unsupported mutation type: %v", mutationType)
		}
	}

	if mg.SourceFSAdapter == nil || mg.GoFileAdapter == nil {
		return nil, fmt.Errorf("missing adapters")
	}

	content, err := mg.ReadFile(source.Origin.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", source.Origin.Path, err)
	}

	fset := token.NewFileSet()
	file, err := mg.Parse(fset, string(source.Origin.Path), content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", source.Origin.Path, err)
	}

	var mutations []m.MutationV2
	mutationID := startingIndex

	for _, mutationType := range mutationTypes {
		ast.Inspect(file, func(n ast.Node) bool {
			switch mutationType {
			case m.MutationArithmetic:
				mutations = append(mutations, mutagens.GenerateArithmeticMutations(n, fset, content, source, &mutationID)...)
			case m.MutationBoolean:
				mutations = append(mutations, mutagens.GenerateBooleanMutations(n, fset, content, source, &mutationID)...)
			}

			return true
		})
	}

	return mutations, nil
}
