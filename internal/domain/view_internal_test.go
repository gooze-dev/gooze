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

	root := rpt.Source{Origin: &rpt.File{ShortPath: "main.go"}}
	assert.True(t, matchesPaths(root, []m.Path{"."}), "\".\" matches files in the module root")
	assert.False(t, matchesPaths(source, []m.Path{"."}), "\".\" is not recursive")
	assert.False(t, matchesPaths(source, []m.Path{"-x/..."}), "a directory sorting before \".\" is not the root")

	absoluteOnly := rpt.Source{Origin: &rpt.File{FullPath: "/elsewhere/a.go"}}
	assert.False(t, matchesPaths(absoluteOnly, []m.Path{"./..."}), "relative patterns need a module-relative path")
	assert.True(t, matchesPaths(absoluteOnly, []m.Path{"/elsewhere/..."}))
	assert.True(t, matchesPaths(rpt.Source{}, nil))
}

func TestTopFiles(t *testing.T) {
	files := []FileSummary{
		{Path: "a.go", Summary: rpt.Summary{Total: 9, Killed: 9}},
		{Path: "b.go", Summary: rpt.Summary{Total: 9, Killed: 5, Survived: 2, NotCovered: 1, Timeout: 1}},
		{Path: "c.go", Summary: rpt.Summary{Total: 9, Killed: 6, Survived: 3}},
		{Path: "d.go", Summary: rpt.Summary{Total: 9, Killed: 6, Survived: 1, Error: 2}},
		{Path: "e.go", Summary: rpt.Summary{Total: 9, Killed: 6, Survived: 3}},
		{Path: "f.go", Summary: rpt.Summary{Total: 9, Killed: 4, Skipped: 5}},
	}

	assert.Equal(t, []string{"f.go", "b.go", "c.go", "e.go"}, paths(topFiles(files, 4)), "most mutants against the score first, then by path")
	assert.Equal(t, []string{"f.go", "b.go", "c.go", "e.go", "d.go", "a.go"}, paths(topFiles(files, 6)))
	assert.Equal(t, []string{"a.go", "b.go", "c.go", "d.go", "e.go", "f.go"}, paths(files), "the files keep their order")
}

func paths(files []FileSummary) []string {
	out := make([]string, 0, len(files))
	for _, file := range files {
		out = append(out, file.Path)
	}

	return out
}
