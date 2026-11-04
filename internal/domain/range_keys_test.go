package domain_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
)

// rangeKeysSrc is the case of #123: a range key compared with 0 and with
// len-1 of the slice it ranges over.
const rangeKeysSrc = `package p

func Join(paragraphs []string, counts map[int]int) string {
	out := ""
	for i, para := range paragraphs {
		if i == 0 {
			out += para
		}
		if i < len(paragraphs)-1 {
			out += "\n"
		}
	}
	for k := range counts {
		if k == 0 {
			out += "zero"
		}
	}
	return out
}
`

// mutatedConditions returns the conditions of the if statements on lines the
// mutations change.
func mutatedConditions(t *testing.T, withTypes bool) []string {
	t.Helper()

	var conditions []string

	for _, mutation := range streamWithTypes(t, rangeKeysSrc, withTypes, mgen.Comparison) {
		line := strings.Split(string(mutation.MutatedCode), "\n")[mutation.Line-1]
		conditions = append(conditions, strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(line), "if "), " {"))
	}

	return conditions
}

func TestMutagen_RangeKeysGetNoEquivalentComparisonMutants(t *testing.T) {
	withTypes := mutatedConditions(t, true)

	assert.NotContains(t, withTypes, "i <= 0", "a slice's key is never negative")
	assert.NotContains(t, withTypes, "i != len(paragraphs)-1", "nor reaches its length")
	assert.Contains(t, withTypes, "i >= 0")
	assert.Contains(t, withTypes, "k <= 0", "a map's key can be negative")

	without := mutatedConditions(t, false)

	assert.Contains(t, without, "i <= 0", "without type information every mutant stays")
	assert.Contains(t, without, "i != len(paragraphs)-1")
}
