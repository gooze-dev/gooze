package domain_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	m "gooze.dev/pkg/gooze/internal/model"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
)

// uncompilableSrc has mutants that can't build: a constant divisor made 0,
// a case value made equal to another case, and an ordering comparison of
// operands without an order.
const uncompilableSrc = `package p

type point struct{ x int }

func Div(n int, f float64) (int, int, float64) {
	q := n / 10
	r := n % (2 * 5)
	n /= 3
	f /= 4
	return q + r + n, 8 / 4, f / 2
}

func Kind(n int) string {
	switch n {
	case 1:
		return "one"
	case 7:
		return "seven"
	}
	return ""
}

func Same(a, b point, p *int, x, y int) bool {
	return a == b && p != nil && x == y
}

func Other(n int, e any) (int, bool) {
	n *= 3
	n &= 3
	return n*3 + n&3, 1 == e || e == 1
}
`

// changedLines returns, for each mutant, its line and the trimmed text of
// that line in the mutated code.
func changedLines(mutations []m.Mutation) []string {
	var out []string

	for _, mutation := range mutations {
		line := strings.Split(string(mutation.MutatedCode), "\n")[mutation.Line-1]
		out = append(out, strings.TrimSpace(line))
	}

	sort.Strings(out)

	return out
}

func TestMutagen_NumbersSkipsZeroDivisorsAndDuplicateCases(t *testing.T) {
	kept := []string{
		"case 0:", // case 1 -> 0
		"case 0:", // case 7 -> 0; 7 -> 1 duplicates case 1
		"f /= 0",  // a float divided by 0 builds
		"f /= 1",
		"n /= 1",
		"q := n / 1",
		"r := n % (1 * 5)",
		"r := n % (2 * 1)",
		"return q + r + n, 0 / 4, f / 2",
		"return q + r + n, 1 / 4, f / 2",
		"return q + r + n, 8 / 1, f / 2", // 8 / 0 is a constant division by zero
		"return q + r + n, 8 / 4, f / 0",
		"return q + r + n, 8 / 4, f / 1",
		"n *= 0", // only / and % divide
		"n *= 1",
		"n &= 0",
		"n &= 1",
		"return n*0 + n&3, 1 == e || e == 1",
		"return n*1 + n&3, 1 == e || e == 1",
		"return n*3 + n&0, 1 == e || e == 1",
		"return n*3 + n&1, 1 == e || e == 1",
		"return n*3 + n&3, 0 == e || e == 1",
		"return n*3 + n&3, 1 == e || e == 0",
	}

	t.Run("with types", func(t *testing.T) {
		assert.ElementsMatch(t, kept, changedLines(streamWithTypes(t, uncompilableSrc, true, mgen.Numbers)))
	})

	// Without types, an int dividend isn't known: only the divisions of
	// constants are known to be integer ones.
	t.Run("without types", func(t *testing.T) {
		want := append([]string{"n /= 0", "q := n / 0", "r := n % (0 * 5)", "r := n % (2 * 0)"}, kept...)
		assert.ElementsMatch(t, want, changedLines(streamWithTypes(t, uncompilableSrc, false, mgen.Numbers)))
	})
}

func TestMutagen_ComparisonSkipsOrderingOfUnorderedTypes(t *testing.T) {
	lines := changedLines(streamWithTypes(t, uncompilableSrc, true, mgen.Comparison))

	// a == b, p != nil, 1 == e and e == 1 keep only their != or == mutant;
	// x == y keeps all 5.
	assert.Len(t, lines, 9)

	for _, line := range lines {
		assert.NotRegexp(t, `a [<>]|p [<>]|1 [<>]|e [<>]`, line)
	}

	// Without types, a == b keeps its ordering mutants (p != nil has only
	// its == mutant either way).
	assert.Len(t, streamWithTypes(t, uncompilableSrc, false, mgen.Comparison), 21)
}

func TestMutagen_ArithmeticSkipsZeroDivisors(t *testing.T) {
	// 2 * 5 -> 2 / 5 is 0, so n % (2 / 5) divides by zero; 2 % 5 is 2.
	withTypes := changedLines(streamWithTypes(t, uncompilableSrc, true, mgen.Arithmetic))
	assert.NotContains(t, withTypes, "r := n % (2 / 5)")
	assert.Contains(t, withTypes, "r := n % (2 % 5)")

	// Without types, n isn't known to be an integer.
	assert.Contains(t, changedLines(streamWithTypes(t, uncompilableSrc, false, mgen.Arithmetic)), "r := n % (2 / 5)")
}
