package domain

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const sampleProfile = `mode: set
gooze.dev/pkg/gooze/internal/model/report.go:10.2,12.16 2 1
gooze.dev/pkg/gooze/internal/model/report.go:20.2,22.16 1 0
gooze.dev/pkg/gooze/internal/model/report.go:30.10,35.4 3 1
other/pkg/thing.go:5.2,5.20 1 1
`

func TestParseCoverage_Covers(t *testing.T) {
	idx, err := ParseCoverage([]byte(sampleProfile))
	require.NoError(t, err)
	require.NotNil(t, idx)

	tests := []struct {
		name      string
		shortPath string
		line      int
		want      bool
	}{
		{"covered line at range start", "internal/model/report.go", 10, true},
		{"covered line inside range", "internal/model/report.go", 11, true},
		{"covered line at range end", "internal/model/report.go", 12, true},
		{"uncovered (count 0) block", "internal/model/report.go", 21, false},
		{"line outside all ranges", "internal/model/report.go", 100, false},
		{"second covered multi-line range", "internal/model/report.go", 33, true},
		{"file absent from profile", "internal/other/missing.go", 10, false},
		{"suffix match with leading path", "model/report.go", 11, true},
		{"path with leading slash normalises", "/internal/model/report.go", 11, true},
		{"other file covered", "thing.go", 5, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, idx.Covers(tt.shortPath, tt.line))
		})
	}
}

func TestParseCoverage_Empty(t *testing.T) {
	idx, err := ParseCoverage([]byte("mode: set\n"))
	require.NoError(t, err)
	require.False(t, idx.Covers("anything.go", 1))
}

func TestParseCoverage_Malformed(t *testing.T) {
	// A malformed block line is skipped, valid ones still parsed.
	profile := "mode: set\nnot a valid line\nmod/a.go:1.1,1.5 1 1\n"
	idx, err := ParseCoverage([]byte(profile))
	require.NoError(t, err)
	require.True(t, idx.Covers("a.go", 1))
}

func TestCoverageIndex_Covers_PrefersShortestMatchingPath(t *testing.T) {
	profile := []byte("mode: set\n" +
		"example.com/m/cmd/x/main.go:1.1,50.1 1 1\n" +
		"example.com/m/main.go:1.1,2.1 1 0\n")

	idx, err := ParseCoverage(profile)
	if err != nil {
		t.Fatalf("ParseCoverage() error = %v", err)
	}

	if idx.Covers("main.go", 10) {
		t.Fatal("root main.go line 10 reported covered by the nested cmd/x/main.go entry")
	}

	if !idx.Covers("cmd/x/main.go", 10) {
		t.Fatal("cmd/x/main.go line 10 should be covered")
	}
}

func TestParseCoverage_IgnoresEmptyPath(t *testing.T) {
	idx, err := ParseCoverage([]byte(":1.1,2.1 1 1\n"))
	if err != nil {
		t.Fatalf("ParseCoverage() error = %v", err)
	}

	if len(idx.files) != 0 {
		t.Fatalf("expected no indexed files, got %v", idx.files)
	}
}

func TestCoverageIndex_Uncovered_UnknownLinesAreNotUncovered(t *testing.T) {
	idx, err := ParseCoverage([]byte("mode: set\nmod/a.go:10.1,12.5 2 1\nmod/a.go:14.1,15.5 1 0\n"))
	if err != nil {
		t.Fatalf("ParseCoverage() error = %v", err)
	}

	if idx.Uncovered("a.go", 11) {
		t.Fatal("covered line reported uncovered")
	}

	if !idx.Uncovered("a.go", 14) {
		t.Fatal("line in a zero-count block should be uncovered")
	}

	if idx.Uncovered("a.go", 3) {
		t.Fatal("declaration line outside every block must not be uncovered")
	}

	if idx.Uncovered("other.go", 14) {
		t.Fatal("file missing from the profile must not be uncovered")
	}
}

// The profile of
//
//	10	if a < b {
//	11		return a
//	12	}
//	13	return b
//
// when a < b never held.
const guardProfile = "mode: set\nmod/a.go:10.2,10.11 1 3\nmod/a.go:10.11,12.3 1 0\nmod/a.go:13.2,13.10 1 3\n"

func TestCoverageIndex_UncoveredAt(t *testing.T) {
	idx, err := ParseCoverage([]byte(guardProfile + "mod/b.go:10.11,12.3 1 0\nmod/b.go:10.11,12.3 1 2\nmod/d.go:10.11,12.3 1 0\n"))
	require.NoError(t, err)

	tests := []struct {
		name         string
		path         string
		line, column int
		want         bool
	}{
		{"first statement of the block that never ran", "a.go", 11, 3, true},
		{"start of that block", "a.go", 10, 11, true},
		{"last column of that block", "a.go", 12, 2, true},
		{"end of that block, which is past it", "a.go", 12, 3, false},
		{"the condition, on the line of the block's brace", "a.go", 10, 5, false},
		{"before the block on its first line", "a.go", 10, 10, false},
		{"a block that ran", "a.go", 13, 2, false},
		{"outside every block", "a.go", 3, 1, false},
		{"a file the profile lacks", "c.go", 11, 3, false},
		{"a block that ran in one of its entries", "b.go", 11, 3, false},
		{"before a block that never ran, on its first line", "d.go", 10, 5, false},
		{"after the start of a block on its first line", "d.go", 10, 12, true},
		{"on the last line of a block, left of its first column", "d.go", 12, 1, true},
		{"past a block that never ran", "d.go", 13, 1, false},
		{"the line before a block, right of its first column", "d.go", 9, 20, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, idx.UncoveredAt(tt.path, tt.line, tt.column))
		})
	}

	var none *CoverageIndex

	require.False(t, none.UncoveredAt("a.go", 11, 3))
}

func TestCoverageIndex_NilCoversNothing(t *testing.T) {
	var none *CoverageIndex

	require.False(t, none.Covers("a.go", 11))
	require.False(t, none.Uncovered("a.go", 11))
}

func TestParseCoverage_CountBelowOneIsNotCovered(t *testing.T) {
	idx, err := ParseCoverage([]byte("mode: count\nmod/a.go:10.1,12.5 2 -1\nmod/a.go:14.1,15.5 1 0\n"))
	require.NoError(t, err)

	require.True(t, idx.Uncovered("a.go", 11))
	require.True(t, idx.Uncovered("a.go", 14))
}

func TestParseCoverage_PathOfOneLetter(t *testing.T) {
	idx, err := ParseCoverage([]byte("mode: set\na:10.1,12.5 2 1\n:10.1,12.5 2 1\n"))
	require.NoError(t, err)

	require.Len(t, idx.files, 1)
	require.True(t, idx.Covers("a", 11))
}

func TestCoverageIndex_PathAsTheProfileWritesIt(t *testing.T) {
	idx, err := ParseCoverage([]byte("mode: set\na.go:10.1,12.5 2 1\n"))
	require.NoError(t, err)

	require.True(t, idx.Covers("a.go", 11))
	require.False(t, idx.Covers("ba.go", 11))
}

func TestCoverageIndex_ShortestPathIsTheFile(t *testing.T) {
	// Many entries, so that a pick by map order would fail on some run.
	profile := "mode: set\nmod/a.go:10.1,12.5 2 1\nmod/x/a.go:10.1,12.5 2 0\nmod/y/a.go:10.1,12.5 2 0\n" +
		"mod/z/a.go:10.1,12.5 2 0\nmod/w/a.go:10.1,12.5 2 0\n"

	for range 20 {
		idx, err := ParseCoverage([]byte(profile))
		require.NoError(t, err)

		require.True(t, idx.Covers("a.go", 11))
		require.Equal(t, "mod/w/a.go", idx.resolve("w/a.go"))
		require.Empty(t, idx.resolve("missing.go"))
	}
}

func TestCoverageIndex_SameLengthPathsResolveByName(t *testing.T) {
	idx, err := ParseCoverage([]byte("mode: set\nb/a.go:10.1,12.5 2 0\na/a.go:10.1,12.5 2 1\nc/a.go:10.1,12.5 2 0\n"))
	require.NoError(t, err)

	for range 20 {
		require.Equal(t, "a/a.go", idx.resolve("a.go"))
	}
}
