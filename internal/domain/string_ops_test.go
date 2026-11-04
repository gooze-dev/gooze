package domain_test

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gooze.dev/pkg/gooze/internal/adapter"
	domain "gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
)

// stringOperandsSrc has string and int operands of + and +=.
const stringOperandsSrc = `package p

import "strconv"

func Name(s string, n int) string {
	return s + strconv.Itoa(n)
}

func Sum(a, b int) int {
	return a + b
}

func Join(parts []string) (string, int) {
	out := ""
	total := 0
	for _, p := range parts {
		out += p
		total += 1
	}
	return out, total
}
`

// streamWithTypes generates the mutations of src, in a module of its own, by
// a mutagen with types when withTypes is set.
func streamWithTypes(t *testing.T, src string, withTypes bool, mutationType mgen.Type) []m.Mutation {
	t.Helper()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/p\n\ngo 1.24\n"), 0o600))

	path := filepath.Join(dir, "p.go")
	require.NoError(t, os.WriteFile(path, []byte(src), 0o600))

	var opts []domain.MutagenOption
	if withTypes {
		// Its own cache: gocache's tests check that the shared one is left alone.
		types := adapter.NewExprTypes()
		types.SetBuildEnv([]string{"GOCACHE=" + t.TempDir()})
		opts = append(opts, domain.WithExprTypes(types))
	}

	mg := domain.NewMutagen(adapter.NewLocalGoFileAdapter(), adapter.NewLocalSourceFSAdapter(), adapter.NewUnifiedDiffer(), opts...)

	mutations, err := mg.GenerateMutation(context.Background(), makeSourceV2(t, path), mutationType)
	require.NoError(t, err)

	return mutations
}

func TestMutagen_StringOperandsGetOnlyMutantsThatBuild(t *testing.T) {
	cases := map[string]struct {
		mutationType mgen.Type
		withTypes    bool
		lines        []int
	}{
		"arithmetic: string + has none": {mutationType: mgen.Arithmetic, withTypes: true, lines: []int{10, 10, 10, 10}},
		"assignment: string += keeps =": {mutationType: mgen.Assignment, withTypes: true, lines: []int{17, 18, 18}},
		"arithmetic without types":      {mutationType: mgen.Arithmetic, lines: []int{6, 6, 6, 6, 10, 10, 10, 10}},
		"assignment without types":      {mutationType: mgen.Assignment, lines: []int{17, 17, 18, 18}},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			lines := linesOf(streamWithTypes(t, stringOperandsSrc, tc.withTypes, tc.mutationType))
			sort.Ints(lines)

			assert.Equal(t, tc.lines, lines)
		})
	}
}

func TestMutagen_StringAssignmentKeepsTheAssignMutant(t *testing.T) {
	mutations := streamWithTypes(t, stringOperandsSrc, true, mgen.Assignment)

	var codes []string

	for _, mutation := range mutations {
		if mutation.Line == 17 {
			codes = append(codes, strings.TrimSpace(strings.Split(string(mutation.MutatedCode), "\n")[16]))
		}
	}

	assert.Equal(t, []string{"out = p"}, codes)
}
