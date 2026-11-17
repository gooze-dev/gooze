package domain_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	domain "gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

func survivorAt(path string, line int, mutagen string) domain.Survivor {
	return domain.Survivor{Path: path, Line: line, Mutagen: mutagen, Status: rpt.Survived}
}

// listed renders what ListedLines keeps as "path: line×mutants ... +hidden".
func listed(files []domain.FileLines) []string {
	out := make([]string, 0, len(files))

	for _, file := range files {
		text := file.Path + ":"
		for _, line := range file.Lines {
			text += fmt.Sprintf(" %d×%d", line.Line, len(line.Survivors))
			_ = line.Path
		}

		out = append(out, fmt.Sprintf("%s +%d", text, file.Hidden))
	}

	return out
}

func TestListedLines(t *testing.T) {
	survivors := []domain.Survivor{
		survivorAt("a.go", 3, "comparison"),
		survivorAt("a.go", 3, "numbers"),
		survivorAt("a.go", 9, "boolean"),
		survivorAt("a.go", 12, "boolean"),
		survivorAt("b.go", 3, "boolean"),
		survivorAt("c.go", 1, "boolean"),
		survivorAt("c.go", 2, "boolean"),
	}

	for name, tt := range map[string]struct {
		limit int
		want  []string
	}{
		"no cap":            {limit: 0, want: []string{"a.go: 3×2 9×1 12×1 +0", "b.go: 3×1 +0", "c.go: 1×1 2×1 +0"}},
		"one line":          {limit: 1, want: []string{"a.go: 3×2 +2", "b.go: 3×1 +0", "c.go: 1×1 +1"}},
		"two lines":         {limit: 2, want: []string{"a.go: 3×2 9×1 +1", "b.go: 3×1 +0", "c.go: 1×1 2×1 +0"}},
		"as many as a file": {limit: 3, want: []string{"a.go: 3×2 9×1 12×1 +0", "b.go: 3×1 +0", "c.go: 1×1 2×1 +0"}},
		"more than any":     {limit: 9, want: []string{"a.go: 3×2 9×1 12×1 +0", "b.go: 3×1 +0", "c.go: 1×1 2×1 +0"}},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tt.want, listed(domain.ListedLines(survivors, tt.limit)))
		})
	}

	assert.Empty(t, domain.ListedLines(nil, 2))
}

// cappedFixture writes reports with survivors on lines 2, 7 and 20 of
// pkg/a.go and on line 2 of pkg/b.go.
func cappedFixture(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	a := rpt.Source{Origin: &rpt.File{ShortPath: "pkg/a.go", FullPath: "/p/pkg/a.go", Hash: "a"}}
	b := rpt.Source{Origin: &rpt.File{ShortPath: "pkg/b.go", FullPath: "/p/pkg/b.go", Hash: "b"}}

	report := func(source rpt.Source, id string, line int) m.Report {
		diff := []byte(fmt.Sprintf("--- original\n+++ mutated\n@@ -%[1]d,1 +%[1]d,1 @@\n-a\n+b\n", line))

		return m.Report{Source: source, Result: m.Result{mgen.Boolean: {{MutationID: id, Status: rpt.Survived}}}, Diff: &diff}
	}

	writeReports(t, dir, report(a, "a-1", 2), report(a, "a-2", 7), report(a, "a-3", 20), report(b, "b-1", 2))

	return dir
}

func TestWorkflow_View_MaxPerFileCapsTheAnnotations(t *testing.T) {
	const annotation = "::warning file=pkg/%s,line=%d,title=Surviving mutant (boolean)::Tests still pass with `a` changed to `b`\n"

	for name, tt := range map[string]struct {
		limit int
		want  string
	}{
		"every line": {limit: 0, want: fmt.Sprintf(annotation, "a.go", 2) + fmt.Sprintf(annotation, "a.go", 7) +
			fmt.Sprintf(annotation, "a.go", 20) + fmt.Sprintf(annotation, "b.go", 2)},
		"two lines": {limit: 2, want: fmt.Sprintf(annotation, "a.go", 2) + fmt.Sprintf(annotation, "a.go", 7) +
			"::notice file=pkg/a.go,title=1 more line with mutants::report.max_per_file lists 2 per file\n" +
			fmt.Sprintf(annotation, "b.go", 2)},
		"one line": {limit: 1, want: fmt.Sprintf(annotation, "a.go", 2) +
			"::notice file=pkg/a.go,title=2 more lines with mutants::report.max_per_file lists 1 per file\n" +
			fmt.Sprintf(annotation, "b.go", 2)},
	} {
		t.Run(name, func(t *testing.T) {
			out, err := viewFormatted(t, domain.ViewArgs{Reports: m.Path(cappedFixture(t)), Format: domain.FormatGitHub, MaxPerFile: tt.limit}, true)
			require.NoError(t, err)
			assert.Equal(t, tt.want, out)
		})
	}
}

func TestWorkflow_View_MaxPerFileShapesOnlyTheOutput(t *testing.T) {
	summary := viewSummary(t, domain.ViewArgs{Reports: m.Path(cappedFixture(t)), MaxPerFile: 1})

	assert.Equal(t, 1, summary.MaxPerFile)
	assert.Len(t, summary.Survivors, 4, "the summary keeps every survivor: the cap is on what is listed")
	assert.Equal(t, 4, summary.Score.Survived)
	assert.Zero(t, viewSummary(t, domain.ViewArgs{Reports: m.Path(cappedFixture(t))}).MaxPerFile)
}

func TestWorkflow_View_MaxPerFileKeepsMachineFormatsAndGatesWhole(t *testing.T) {
	out, err := viewFormatted(t, domain.ViewArgs{
		Reports: m.Path(cappedFixture(t)), Format: domain.FormatSARIF, MaxPerFile: 1,
		Thresholds: domain.Thresholds{Key: "threshold", MaxSurvivors: 3},
	}, false)

	require.Error(t, err, "four survivors are over the cap of three, listed or not")
	assert.Equal(t, m.ExitBelowThreshold, exitCodeOf(err))

	for _, line := range []string{`"startLine": 2`, `"startLine": 7`, `"startLine": 20`} {
		assert.Contains(t, out, line)
	}
}
