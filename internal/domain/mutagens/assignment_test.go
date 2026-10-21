package mutagens

import (
	"testing"

	"github.com/stretchr/testify/assert"

	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
)

func TestGenerateAssignmentMutations(t *testing.T) {
	cases := map[string][]string{
		"n += a":                           {"n -= a", "n = a"},
		"n -= a":                           {"n += a", "n = a"},
		"n *= a":                           {"n /= a", "n = a"},
		"n /= a":                           {"n *= a", "n = a"},
		"n %= a":                           {"n *= a", "n = a"},
		"n &= a":                           {"n = a", "n |= a"},
		"n |= a":                           {"n &= a", "n = a"},
		"n ^= a":                           {"n &= a", "n = a"},
		"n &^= a":                          {"n &= a", "n = a"},
		"n <<= a":                          {"n = a", "n >>= a"},
		"n >>= a":                          {"n <<= a", "n = a"},
		`s += "x"`:                         {`s = "x"`},
		"switch {\ncase a > b:\nn += a\n}": {"n -= a", "n = a"},
		"for i := 0; i < n; i += 2 {\n}":   nil,
		"n = a":                            nil,
		"x := a\n_ = x":                    nil,
	}

	for body, want := range cases {
		assert.Equal(t, want, mutantsOf(t, GenerateAssignmentMutations, body), body)
	}
}

func TestGenerateAssignmentMutations_Type(t *testing.T) {
	code := "package p\n\nfunc f(n int) {\n\tn += 2\n}\n"

	for _, mutation := range generateAll(t, GenerateAssignmentMutations, code) {
		assert.Equal(t, mgen.Assignment, mutation.Type)
		assert.Equal(t, mutationID(mutation.MutatedCode), mutation.ID)
	}
}
