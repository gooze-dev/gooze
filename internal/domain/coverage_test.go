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
