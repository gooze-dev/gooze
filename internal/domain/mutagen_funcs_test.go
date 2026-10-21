package domain_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
)

func TestMutagen_RecordsEnclosingFunction(t *testing.T) {
	src := `package calc

var limit = 10 + 1

func InRange(n, lo, hi int) bool {
	inside := func() bool { return n < lo }
	return inside() || n > hi
}

type Parser struct{ pos int }

func (p *Parser) Next() int { return p.pos + 1 }

func (p Parser) Peek() int { return p.pos - 1 }

type Gen[T any] struct{}

func (g Gen[T]) Do() int { return 2 * 3 }
`
	path := filepath.Join(t.TempDir(), "calc.go")
	require.NoError(t, os.WriteFile(path, []byte(src), 0o600))

	mutations, err := newTestMutagen().GenerateMutation(context.Background(), makeSourceV2(t, path), mgen.Arithmetic, mgen.Comparison)
	require.NoError(t, err)

	byLine := map[int][]string{}
	for _, mutation := range mutations {
		byLine[mutation.Line] = mutation.Funcs
	}

	assert.Equal(t, map[int][]string{
		3:  nil,         // package level: no function
		6:  {"InRange"}, // a closure belongs to its function
		7:  {"InRange"},
		12: {"Next", "Parser.Next"}, // pointer receiver
		14: {"Peek", "Parser.Peek"}, // value receiver
		18: {"Do", "Gen.Do"},        // generic receiver, without type parameters
	}, byLine)
}
