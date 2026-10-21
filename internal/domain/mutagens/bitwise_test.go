package mutagens

import (
	"go/token"
	"testing"

	"github.com/stretchr/testify/assert"

	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
)

func TestGenerateBitwiseMutations(t *testing.T) {
	cases := map[string][]string{
		"n = a & b":  {"n = a | b"},
		"n = a | b":  {"n = a & b"},
		"n = a ^ b":  {"n = a & b"},
		"n = a &^ b": {"n = a & b"},
		"n = a << b": {"n = a >> b"},
		"n = a >> b": {"n = a << b"},
		"n = a&b | a&^b": {
			"n = a&b & a&^b",
			"n = a&b | a&b",
			"n = a|b | a&^b",
		},
		"n = a + b":  nil,
		"n = ^a":     nil,
		"_ = a == b": nil,
	}

	for body, want := range cases {
		assert.Equal(t, want, mutantsOf(t, GenerateBitwiseMutations, body), body)
	}
}

func TestGenerateBitwiseMutations_Type(t *testing.T) {
	code := "package p\n\nvar n = 1 << 2\n"

	for _, mutation := range generateAll(t, GenerateBitwiseMutations, code) {
		assert.Equal(t, mgen.Bitwise, mutation.Type)
		assert.Equal(t, mutationID(mutation.MutatedCode), mutation.ID)
	}
}

func TestIsBitwiseOp(t *testing.T) {
	for _, op := range []token.Token{token.AND, token.OR, token.XOR, token.AND_NOT, token.SHL, token.SHR} {
		assert.True(t, isBitwiseOp(op), op.String())
	}

	for _, op := range []token.Token{token.ADD, token.LAND, token.LOR, token.EQL, token.AND_ASSIGN} {
		assert.False(t, isBitwiseOp(op), op.String())
	}
}
