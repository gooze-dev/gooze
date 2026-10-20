package report

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDiffChanges(t *testing.T) {
	tests := []struct {
		name string
		diff string
		want []Change
	}{
		{
			name: "replaced line after context",
			diff: "--- original\n+++ mutated\n@@ -7,7 +7,7 @@\n \n // Clamp limits n.\n func Clamp(n, lo, hi int) int {\n-\tif n < lo {\n+\tif n <= lo {\n \t\treturn lo\n \t}\n",
			want: []Change{{Line: 10, Original: "\tif n < lo {", Mutated: "\tif n <= lo {"}},
		},
		{
			name: "removed line",
			diff: "--- original\n+++ mutated\n@@ -3,3 +3,2 @@\n a()\n-\tb()\n c()\n",
			want: []Change{{Line: 4, Original: "\tb()"}},
		},
		{
			name: "multiple hunks, blank context and no newline marker",
			diff: "--- original\n+++ mutated\n@@ -1,3 +1,3 @@\n a\n-b\n+B\n\n@@ -10,2 +10,2 @@\n-x\n+X\n y\n\\ No newline at end of file\n",
			want: []Change{{Line: 2, Original: "b", Mutated: "B"}, {Line: 10, Original: "x", Mutated: "X"}},
		},
		{
			name: "single-line hunk header",
			diff: "--- original\n+++ mutated\n@@ -5 +5 @@\n-x := 1\n+x := 0\n",
			want: []Change{{Line: 5, Original: "x := 1", Mutated: "x := 0"}},
		},
		{
			name: "added line without a removal",
			diff: "--- original\n+++ mutated\n@@ -1,2 +1,3 @@\n a\n+inserted\n b\n",
		},
		{name: "hunk starting at line zero", diff: "--- original\n+++ mutated\n@@ -0,0 +1 @@\n-x\n+y\n"},
		{name: "empty", diff: ""},
		{name: "headers only", diff: "--- original\n+++ mutated\n"},
		{name: "malformed hunk", diff: "@@ garbage @@\n-x\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, DiffChanges([]byte(tt.diff)))
		})
	}
}
