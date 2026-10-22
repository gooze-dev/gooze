package domain

import (
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"

	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
)

func TestMutationGenerators_CoverRegistry(t *testing.T) {
	require.Len(t, mutationGenerators, len(mgen.All()))

	for _, mg := range mgen.All() {
		require.Contains(t, mutationGenerators, mg.Type, mg.Name)
	}
}

func TestReplacementOf(t *testing.T) {
	tests := []struct{ original, mutated, want string }{
		{"a < b", "a != b", "!="},
		{"a < b", "a <= b", "<="},
		{"a <= b", "a < b", "<"},
		{"a == b", "a <= b", "<="},
		{"a != b", "a == b", "=="},
		{"a + b", "a - b", "-"},
		{"a - b", "a + b", "+"},
		{"c += n", "c = n", "="},
		{"c++", "c--", "--"},
		{"x := 10", "x := 0", "0"},
		{"x := 5", "x := 1", "1"},
		{"return true", "return false", "false"},
		{"y := 1.5", "y := 0.0", "0.0"},
		{"a < -b", "a <= -b", "<="},
		{"same", "same", ""},
		{"a <=", "a <", "<"},
		{"@ a", "@ b", "b"},
		{"a<bc", "a<b", ";"},
	}

	for _, tt := range tests {
		if got := replacementOf([]byte(tt.original), []byte(tt.mutated)); got != tt.want {
			t.Errorf("replacementOf(%q, %q) = %q, want %q", tt.original, tt.mutated, got, tt.want)
		}
	}
}

func TestTokenAt(t *testing.T) {
	tests := []struct {
		name   string
		src    string
		offset int
		want   string
	}{
		{"operator", "a <= b", 2, "<="},
		{"inside a token", "a <= b", 3, "<="},
		{"before the next token", "a <= b", 4, "<="},
		{"first token", "a <= b", 0, "a"},
		{"no difference", "a <= b", -1, ""},
		{"literal", "x := 0.5", 6, "0.5"},
		{"illegal character", "@ b", 0, "ILLEGAL"},
		// Every byte is a token, and the semicolon Go inserts at the end of
		// the file starts at offset: the scan takes len(src)+1 tokens.
		{"inserted semicolon at the end", "a<b", 3, ";"},
		{"past the end", "a<b", 10, ";"},
		{"empty", "", 0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tokenAt([]byte(tt.src), tt.offset))
		})
	}
}

func TestTokenText(t *testing.T) {
	require.Equal(t, "0.5", tokenText(token.FLOAT, "0.5"))
	require.Equal(t, "x", tokenText(token.IDENT, "x"))
	require.Equal(t, "<=", tokenText(token.LEQ, ""))
	require.Equal(t, ";", tokenText(token.SEMICOLON, "\n"))
}
