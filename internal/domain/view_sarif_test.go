package domain_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	domain "gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

type sarifLog struct {
	Schema  string `json:"$schema"`
	Version string `json:"version"`
	Runs    []struct {
		Tool struct {
			Driver struct {
				Name           string `json:"name"`
				Version        string `json:"version"`
				InformationURI string `json:"informationUri"`
				Rules          []struct {
					ID               string `json:"id"`
					ShortDescription struct {
						Text string `json:"text"`
					} `json:"shortDescription"`
					HelpURI string `json:"helpUri"`
				} `json:"rules"`
			} `json:"driver"`
		} `json:"tool"`
		Results []struct {
			RuleID  string `json:"ruleId"`
			Level   string `json:"level"`
			Message struct {
				Text string `json:"text"`
			} `json:"message"`
			Locations []struct {
				PhysicalLocation struct {
					ArtifactLocation struct {
						URI       string `json:"uri"`
						URIBaseID string `json:"uriBaseId"`
					} `json:"artifactLocation"`
					Region *struct {
						StartLine int `json:"startLine"`
					} `json:"region"`
				} `json:"physicalLocation"`
			} `json:"locations"`
			PartialFingerprints map[string]string `json:"partialFingerprints"`
			Properties          struct {
				MutationIDs []string `json:"mutationIds"`
				Mutagens    []string `json:"mutagens"`
			} `json:"properties"`
		} `json:"results"`
	} `json:"runs"`
}

func decodeSARIF(t *testing.T, out string) sarifLog {
	t.Helper()

	var log sarifLog
	require.NoError(t, json.Unmarshal([]byte(out), &log), out)
	require.Len(t, log.Runs, 1)

	return log
}

func fingerprint(ids string) string {
	sum := sha256.Sum256([]byte(ids))
	return hex.EncodeToString(sum[:])
}

func TestWorkflow_View_SARIFFormat(t *testing.T) {
	dir := viewFixture(t)
	diff := []byte(survivorDiff)
	a := rpt.Source{Origin: &rpt.File{ShortPath: "pkg/a.go", FullPath: "/p/pkg/a.go", Hash: "a"}}
	// A second survivor on pkg/a.go:10 combines with a-2 into one result.
	writeReports(t, dir, m.Report{Source: a, Result: m.Result{mgen.Comparison: {{MutationID: "a-0", Status: rpt.Survived}}}, Diff: &diff})

	out, err := viewFormatted(t, domain.ViewArgs{
		Reports:     m.Path(dir),
		Format:      domain.FormatSARIF,
		PathPrefix:  "sub/",
		ToolVersion: "v1.3.0",
	}, false)
	require.NoError(t, err)

	log := decodeSARIF(t, out)
	assert.Contains(t, out, "`if n < lo {` changed to `if n <= lo {`", "code in messages isn't HTML-escaped")
	assert.Equal(t, "https://json.schemastore.org/sarif-2.1.0.json", log.Schema)
	assert.Equal(t, "2.1.0", log.Version)

	driver := log.Runs[0].Tool.Driver
	assert.Equal(t, "gooze", driver.Name)
	assert.Equal(t, "v1.3.0", driver.Version)
	assert.Equal(t, "https://gooze.dev", driver.InformationURI)

	ruleIDs := make([]string, 0, len(driver.Rules))
	for _, rule := range driver.Rules {
		ruleIDs = append(ruleIDs, rule.ID)
		assert.NotEmpty(t, rule.ShortDescription.Text)
		assert.NotEmpty(t, rule.HelpURI)
	}

	assert.Equal(t, []string{"gooze/survived", "gooze/not-covered", "gooze/timeout", "gooze/skipped"}, ruleIDs)

	results := log.Runs[0].Results
	require.Len(t, results, 2)

	uncovered := results[0]
	assert.Equal(t, "gooze/not-covered", uncovered.RuleID)
	assert.Equal(t, "note", uncovered.Level)
	assert.Equal(t, "No test covers this boolean mutation", uncovered.Message.Text)
	assert.Equal(t, "sub/cmd/b.go", uncovered.Locations[0].PhysicalLocation.ArtifactLocation.URI)
	assert.Equal(t, "%SRCROOT%", uncovered.Locations[0].PhysicalLocation.ArtifactLocation.URIBaseID)
	assert.Nil(t, uncovered.Locations[0].PhysicalLocation.Region, "no line without a diff")
	assert.Equal(t, map[string]string{"goozeMutationIds/v1": fingerprint("b-1")}, uncovered.PartialFingerprints)

	line := results[1]
	assert.Equal(t, "gooze/survived", line.RuleID)
	assert.Equal(t, "warning", line.Level)
	assert.Equal(t, "Tests still pass with `if n < lo {` changed to `if n <= lo {`\n"+
		"Tests still pass with `if n < lo {` changed to `if n <= lo {`", line.Message.Text)
	assert.Equal(t, "sub/pkg/a.go", line.Locations[0].PhysicalLocation.ArtifactLocation.URI)
	assert.Equal(t, 10, line.Locations[0].PhysicalLocation.Region.StartLine)
	assert.Equal(t, map[string]string{"goozeMutationIds/v1": fingerprint("a-0,a-2")}, line.PartialFingerprints)
	assert.Equal(t, []string{"a-0", "a-2"}, line.Properties.MutationIDs)
	assert.Equal(t, []string{"comparison"}, line.Properties.Mutagens)
}

func TestWorkflow_View_SARIFFormatWithoutSurvivorsHasEmptyResults(t *testing.T) {
	out, err := viewFormatted(t, domain.ViewArgs{Reports: m.Path(t.TempDir()), Format: domain.FormatSARIF}, false)
	require.NoError(t, err)

	assert.Contains(t, out, `"results": []`)
	assert.Empty(t, decodeSARIF(t, out).Runs[0].Results)
}

func TestWorkflow_View_SARIFFormatKeepsThresholds(t *testing.T) {
	out, err := viewFormatted(t, domain.ViewArgs{
		Reports:    m.Path(viewFixture(t)),
		Format:     domain.FormatSARIF,
		Thresholds: domain.Thresholds{Key: "threshold", Total: 100},
	}, false)

	require.Error(t, err)
	assert.Equal(t, m.ExitBelowThreshold, exitCodeOf(err))
	assert.Len(t, decodeSARIF(t, out).Runs[0].Results, 2)
}
