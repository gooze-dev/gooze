package domain_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	m "gooze.dev/pkg/gooze/internal/model"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
)

// guardsByLine returns, for each line of src with mutants, the position of
// the code its mutants guard, or nil when they guard none.
func guardsByLine(t *testing.T, src string, types ...mgen.Type) map[int]*m.Position {
	t.Helper()

	path := filepath.Join(t.TempDir(), "p.go")
	require.NoError(t, os.WriteFile(path, []byte(src), 0o600))

	mutations, err := newTestMutagen().GenerateMutation(context.Background(), makeSourceV2(t, path), types...)
	require.NoError(t, err)

	guards := map[int]*m.Position{}

	for _, mutation := range mutations {
		if previous, seen := guards[mutation.Line]; seen {
			require.Equal(t, previous, mutation.Guards, "line %d", mutation.Line)
		}

		guards[mutation.Line] = mutation.Guards
	}

	return guards
}

const guardedConditions = `package p

func f(a, b int) int {
	if a < b {
		return a + b
	}

	if a > b {
		return 1
	} else if a > 2 {
		return 2
	}

	for a < 10 {
		a++
	}

	switch {
	case a > 3, b > 3:
		return 3
	case a > 5:
		return 5
	default:
		return 6
	}

	if a < 4 {
	}

	return a - b
}
`

func TestMutagen_ConditionMutantsKnowTheBlockTheyGuard(t *testing.T) {
	guards := guardsByLine(t, guardedConditions, mgen.Comparison, mgen.Arithmetic)

	assert.Equal(t, map[int]*m.Position{
		4:  {Line: 5, Column: 3},
		5:  nil, // a statement of the block, not its condition
		8:  {Line: 9, Column: 3},
		10: {Line: 11, Column: 3},
		14: {Line: 15, Column: 3},
		19: {Line: 20, Column: 3},
		21: {Line: 22, Column: 3},
		27: nil, // an empty block has no code to cover
		30: nil,
	}, guards)
}

const guardedNested = `package p

func f(a, b int, g func(func() bool) bool) int {
	if g(func() bool {
		if a < b {
			return true
		}

		return a > b
	}) {
		return 1
	}

	return 0
}
`

func TestMutagen_ConditionMutantGuardsItsInnermostBlock(t *testing.T) {
	guards := guardsByLine(t, guardedNested, mgen.Comparison)

	assert.Equal(t, map[int]*m.Position{
		5: {Line: 6, Column: 4},
		9: {Line: 11, Column: 3},
	}, guards)
}

const guardedFromItsStart = `package p

//line other.go:100
func f(a int) int {
	if 1 < a {
		return a
	}

	return 0
}
`

func TestMutagen_GuardsFromTheFirstByteOfACondition(t *testing.T) {
	guards := guardsByLine(t, guardedFromItsStart, mgen.Numbers)

	// The position in the file itself, whatever a //line comment says.
	assert.Equal(t, &m.Position{Line: 6, Column: 3}, guards[5])
}
