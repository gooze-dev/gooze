package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"

	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// sarifStatuses are the scored statuses in order of severity, each with the
// SARIF rule its lines report under.
var sarifStatuses = []struct {
	status rpt.TestStatus
	rule   string
	text   string
}{
	{rpt.Survived, "gooze/survived", "Mutants the tests don't catch"},
	{rpt.NotCovered, "gooze/not-covered", "Mutants on lines no test runs"},
	{rpt.Timeout, "gooze/timeout", "Mutants whose tests time out"},
	{rpt.Skipped, "gooze/skipped", "Mutants that weren't tested"},
}

const sarifHelpURI = "https://gooze.dev/docs/usage/results/"

type sarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	Version        string      `json:"version,omitempty"`
	InformationURI string      `json:"informationUri"`
	Rules          []sarifRule `json:"rules"`
}

type sarifRule struct {
	ID               string       `json:"id"`
	ShortDescription sarifMessage `json:"shortDescription"`
	HelpURI          string       `json:"helpUri"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifResult struct {
	RuleID              string            `json:"ruleId"`
	Level               string            `json:"level"`
	Message             sarifMessage      `json:"message"`
	Locations           []sarifLocation   `json:"locations"`
	PartialFingerprints map[string]string `json:"partialFingerprints"`
	Properties          sarifProperties   `json:"properties"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Region           *sarifRegion          `json:"region,omitempty"`
}

type sarifArtifactLocation struct {
	URI       string `json:"uri"`
	URIBaseID string `json:"uriBaseId"`
}

type sarifRegion struct {
	StartLine int `json:"startLine"`
}

type sarifProperties struct {
	MutationIDs []string `json:"mutationIds"`
	Mutagens    []string `json:"mutagens"`
}

// writeSARIF writes a SARIF 2.1.0 log with one result per line with survivors,
// reported under the rule of its most severe mutant.
func writeSARIF(w io.Writer, survivors []Survivor, prefix, version string) error {
	run := sarifRun{
		Tool: sarifTool{Driver: sarifDriver{
			Name:           "gooze",
			Version:        version,
			InformationURI: "https://gooze.dev",
			Rules:          sarifRules(),
		}},
		Results: []sarifResult{},
	}

	for _, line := range groupByLine(survivors) {
		run.Results = append(run.Results, sarifLineResult(line, prefix))
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	// Messages quote Go code: keep its < and > readable instead of \u003c.
	encoder.SetEscapeHTML(false)

	if err := encoder.Encode(sarifLog{Schema: "https://json.schemastore.org/sarif-2.1.0.json", Version: "2.1.0", Runs: []sarifRun{run}}); err != nil {
		return fmt.Errorf("write SARIF: %w", err)
	}

	return nil
}

func sarifRules() []sarifRule {
	rules := make([]sarifRule, 0, len(sarifStatuses))
	for _, s := range sarifStatuses {
		rules = append(rules, sarifRule{ID: s.rule, ShortDescription: sarifMessage{Text: s.text}, HelpURI: sarifHelpURI})
	}

	return rules
}

// lineRule returns the rule of the line's most severe mutant.
func lineRule(line survivorLine) string {
	for _, s := range sarifStatuses {
		if slices.ContainsFunc(line.Survivors, func(survivor Survivor) bool { return survivor.Status == s.status }) {
			return s.rule
		}
	}

	return ""
}

func sarifLineResult(line survivorLine, prefix string) sarifResult {
	level := "note"
	if lineSurvived(line) {
		level = "warning"
	}

	location := sarifPhysicalLocation{ArtifactLocation: sarifArtifactLocation{URI: prefix + line.Path, URIBaseID: "%SRCROOT%"}}
	if line.Line > 0 {
		location.Region = &sarifRegion{StartLine: line.Line}
	}

	var properties sarifProperties

	for _, survivor := range line.Survivors {
		properties.MutationIDs = append(properties.MutationIDs, survivor.MutationID)
		if !slices.Contains(properties.Mutagens, survivor.Mutagen) {
			properties.Mutagens = append(properties.Mutagens, survivor.Mutagen)
		}
	}

	slices.Sort(properties.MutationIDs)

	return sarifResult{
		RuleID:              lineRule(line),
		Level:               level,
		Message:             sarifMessage{Text: lineMessage(line)},
		Locations:           []sarifLocation{{PhysicalLocation: location}},
		PartialFingerprints: map[string]string{"goozeMutationIds/v1": fingerprintOf(properties.MutationIDs)},
		Properties:          properties,
	}
}

// fingerprintOf identifies the mutants of a line across runs: a hash of their
// sorted mutation IDs.
func fingerprintOf(sortedIDs []string) string {
	sum := sha256.Sum256([]byte(strings.Join(sortedIDs, ",")))

	return hex.EncodeToString(sum[:])
}
