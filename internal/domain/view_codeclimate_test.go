package domain_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	domain "gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

type codeClimateIssue struct {
	Type        string   `json:"type"`
	CheckName   string   `json:"check_name"`
	Description string   `json:"description"`
	Categories  []string `json:"categories"`
	Severity    string   `json:"severity"`
	Fingerprint string   `json:"fingerprint"`
	Location    struct {
		Path  string `json:"path"`
		Lines struct {
			Begin int `json:"begin"`
		} `json:"lines"`
	} `json:"location"`
}

func decodeCodeClimate(t *testing.T, out string) []codeClimateIssue {
	t.Helper()

	var issues []codeClimateIssue
	require.NoError(t, json.Unmarshal([]byte(out), &issues), out)

	return issues
}

// codeClimateFixture is viewFixture plus a second survivor on pkg/a.go:10.
func codeClimateFixture(t *testing.T) string {
	t.Helper()

	dir := viewFixture(t)
	diff := []byte(survivorDiff)
	a := rpt.Source{Origin: &rpt.File{ShortPath: "pkg/a.go", FullPath: "/p/pkg/a.go", Hash: "a"}}
	writeReports(t, dir, m.Report{Source: a, Result: m.Result{mgen.Comparison: {{MutationID: "a-0", Status: rpt.Survived}}}, Diff: &diff})

	return dir
}

func TestWorkflow_View_CodeClimateFormat(t *testing.T) {
	out, err := viewFormatted(t, domain.ViewArgs{
		Reports:    m.Path(codeClimateFixture(t)),
		Format:     domain.FormatCodeClimate,
		PathPrefix: "sub/",
	}, false)
	require.NoError(t, err)

	assert.Contains(t, out, "`if n < lo {` changed to `if n <= lo {`", "code in descriptions isn't HTML-escaped")

	issues := decodeCodeClimate(t, out)
	require.Len(t, issues, 2)

	uncovered := issues[0]
	assert.Equal(t, "issue", uncovered.Type)
	assert.Equal(t, "gooze/not-covered", uncovered.CheckName)
	assert.Equal(t, "minor", uncovered.Severity)
	assert.Equal(t, []string{"Bug Risk"}, uncovered.Categories)
	assert.Equal(t, "Uncovered mutant (boolean): No test covers this boolean mutation", uncovered.Description)
	assert.Equal(t, "sub/cmd/b.go", uncovered.Location.Path)
	assert.Equal(t, 1, uncovered.Location.Lines.Begin, "a mutation without a line points at the file's first line")
	assert.Equal(t, fingerprint("b-1"), uncovered.Fingerprint)

	line := issues[1]
	assert.Equal(t, "gooze/survived", line.CheckName)
	assert.Equal(t, "major", line.Severity)
	assert.Equal(t, "2 surviving mutants (comparison): "+
		"Tests still pass with `if n < lo {` changed to `if n <= lo {`; "+
		"Tests still pass with `if n < lo {` changed to `if n <= lo {`", line.Description)
	assert.Equal(t, "sub/pkg/a.go", line.Location.Path)
	assert.Equal(t, 10, line.Location.Lines.Begin)
	assert.Equal(t, fingerprint("a-0,a-2"), line.Fingerprint)
}

func TestWorkflow_View_GitLabFormatIsCodeClimate(t *testing.T) {
	dir := codeClimateFixture(t)

	codeClimate, err := viewFormatted(t, domain.ViewArgs{Reports: m.Path(dir), Format: domain.FormatCodeClimate}, false)
	require.NoError(t, err)

	gitLab, err := viewFormatted(t, domain.ViewArgs{Reports: m.Path(dir), Format: domain.FormatGitLab}, false)
	require.NoError(t, err)

	assert.Equal(t, codeClimate, gitLab)
}

func TestWorkflow_View_CodeClimateFormatWithoutSurvivorsIsAnEmptyArray(t *testing.T) {
	out, err := viewFormatted(t, domain.ViewArgs{Reports: m.Path(t.TempDir()), Format: domain.FormatCodeClimate}, false)
	require.NoError(t, err)

	assert.Equal(t, "[]\n", out)
}

func TestWorkflow_View_CodeClimateFormatKeepsThresholds(t *testing.T) {
	out, err := viewFormatted(t, domain.ViewArgs{
		Reports:    m.Path(viewFixture(t)),
		Format:     domain.FormatGitLab,
		Thresholds: domain.Thresholds{Key: "threshold", Total: 100},
	}, false)

	require.Error(t, err)
	assert.Equal(t, m.ExitBelowThreshold, exitCodeOf(err))
	assert.Len(t, decodeCodeClimate(t, out), 2, "the report is complete before the gate fails")
}
