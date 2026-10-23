package domain

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
)

// codeClimateIssue is one issue of a Code Climate report, with the fields
// GitLab Code Quality reads.
type codeClimateIssue struct {
	Type        string              `json:"type"`
	CheckName   string              `json:"check_name"`
	Description string              `json:"description"`
	Categories  []string            `json:"categories"`
	Severity    string              `json:"severity"`
	Fingerprint string              `json:"fingerprint"`
	Location    codeClimateLocation `json:"location"`
}

type codeClimateLocation struct {
	Path  string           `json:"path"`
	Lines codeClimateLines `json:"lines"`
}

type codeClimateLines struct {
	Begin int `json:"begin"`
}

// writeCodeClimate writes a Code Climate report, a JSON array with one issue
// per line with survivors, checked under the rule of its most severe mutant.
func writeCodeClimate(w io.Writer, survivors []Survivor, prefix string) error {
	lines := groupByLine(survivors)
	// Not nil, so no survivors encode as [].
	issues := make([]codeClimateIssue, 0, len(lines))

	for _, line := range lines {
		issues = append(issues, codeClimateLineIssue(line, prefix))
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	// Descriptions quote Go code: keep its < and > readable instead of <.
	encoder.SetEscapeHTML(false)

	if err := encoder.Encode(issues); err != nil {
		return fmt.Errorf("write Code Climate report: %w", err)
	}

	return nil
}

func codeClimateLineIssue(line survivorLine, prefix string) codeClimateIssue {
	severity := "minor"
	if lineSurvived(line) {
		severity = "major"
	}

	ids := make([]string, 0, len(line.Survivors))
	messages := make([]string, 0, len(line.Survivors))

	for _, survivor := range line.Survivors {
		ids = append(ids, survivor.MutationID)
		messages = append(messages, survivorMessage(survivor))
	}

	slices.Sort(ids)

	return codeClimateIssue{
		Type:        "issue",
		CheckName:   lineRule(line),
		Description: lineTitle(line) + ": " + strings.Join(messages, "; "),
		Categories:  []string{"Bug Risk"},
		Severity:    severity,
		Fingerprint: fingerprintOf(ids),
		// GitLab needs a line; a mutation without one points at the file's first.
		Location: codeClimateLocation{Path: prefix + line.Path, Lines: codeClimateLines{Begin: max(line.Line, 1)}},
	}
}
