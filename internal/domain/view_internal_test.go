package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	m "gooze.dev/pkg/gooze/internal/model"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

func TestMatchesPaths(t *testing.T) {
	source := rpt.Source{Origin: &rpt.File{ShortPath: "internal/domain/view.go", FullPath: "/src/gooze/internal/domain/view.go"}}

	tests := []struct {
		patterns []m.Path
		want     bool
	}{
		{patterns: nil, want: true},
		{patterns: []m.Path{"./..."}, want: true},
		{patterns: []m.Path{"..."}, want: true},
		{patterns: []m.Path{"./internal/..."}, want: true},
		{patterns: []m.Path{"internal/domain/..."}, want: true},
		{patterns: []m.Path{"./internal/domain"}, want: true},
		{patterns: []m.Path{"internal/domain/"}, want: true},
		{patterns: []m.Path{"internal/domain/view.go"}, want: true},
		{patterns: []m.Path{"/src/gooze/internal/..."}, want: true},
		{patterns: []m.Path{"/src/gooze/internal/domain/view.go"}, want: true},
		{patterns: []m.Path{"cmd/...", "internal/domain"}, want: true},
		{patterns: []m.Path{"./internal"}, want: false},
		{patterns: []m.Path{"internal/dom/..."}, want: false},
		{patterns: []m.Path{"internal/domain/workflow.go"}, want: false},
		{patterns: []m.Path{"/other/..."}, want: false},
		{patterns: []m.Path{"view.go"}, want: false},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, matchesPaths(source, tt.patterns), "%v", tt.patterns)
	}

	assert.False(t, matchesPaths(rpt.Source{}, []m.Path{"./..."}), "a source without origin matches only without patterns")
	assert.True(t, matchesPaths(rpt.Source{}, nil))
}
