package domain

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFirstDifference(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want int
	}{
		{"identical", "hello", "hello", -1},
		{"differ at start", "abc", "xbc", 0},
		{"differ in middle", "abcdef", "abcXef", 3},
		{"b is shorter (deletion)", "abcdef", "abc", 3},
		{"a is shorter", "abc", "abcdef", 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, firstDifference([]byte(tt.a), []byte(tt.b)))
		})
	}
}

func TestLineForOffset(t *testing.T) {
	content := []byte("line1\nline2\nline3\n")
	require.Equal(t, 1, lineForOffset(content, 0))  // start of line1
	require.Equal(t, 1, lineForOffset(content, 4))  // within line1
	require.Equal(t, 2, lineForOffset(content, 6))  // start of line2
	require.Equal(t, 3, lineForOffset(content, 12)) // start of line3
	require.Equal(t, 3, lineForOffset(content, 16)) // within line3
	require.Equal(t, 4, lineForOffset(content, 99)) // past final newline clamps to end
}
