package domain

import (
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
	}

	for _, tt := range tests {
		if got := replacementOf([]byte(tt.original), []byte(tt.mutated)); got != tt.want {
			t.Errorf("replacementOf(%q, %q) = %q, want %q", tt.original, tt.mutated, got, tt.want)
		}
	}
}
