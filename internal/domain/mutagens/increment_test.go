package mutagens

import (
	"testing"

	"github.com/stretchr/testify/assert"

	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
)

func TestGenerateIncrementMutations(t *testing.T) {
	cases := map[string][]string{
		"n++":                              {"n--"},
		"n--":                              {"n++"},
		"if a > b {\nn++\n}":               {"n--"},
		"switch {\ncase a > b:\nn--\n}":    {"n++"},
		"select {\ncase <-c:\nn++\n}":      {"n--"},
		"for i := 0; i < n; i++ {\n}":      nil,
		"for i := n; i > 0; i-- {\na++\n}": {"a--"},
		"n += 1":                           nil,
	}

	for body, want := range cases {
		assert.Equal(t, want, mutantsOf(t, GenerateIncrementMutations, body), body)
	}
}

func TestGenerateIncrementMutations_Type(t *testing.T) {
	code := "package p\n\nfunc f(n int) {\n\tn++\n}\n"

	for _, mutation := range generateAll(t, GenerateIncrementMutations, code) {
		assert.Equal(t, mgen.Increment, mutation.Type)
		assert.Equal(t, mutationID(mutation.MutatedCode), mutation.ID)
		assert.Equal(t, "package p\n\nfunc f(n int) {\n\tn--\n}\n", string(mutation.MutatedCode))
	}
}
