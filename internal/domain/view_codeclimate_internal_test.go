package domain

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

func TestCodeClimateIssue_MostSevereMutantSetsTheCheck(t *testing.T) {
	line := survivorLine{lineKey: lineKey{Path: "a.go", Line: 7}, Survivors: []Survivor{
		{MutationID: "z-1", Mutagen: "boolean", Status: rpt.Timeout},
		{MutationID: "a-9", Mutagen: "arithmetic", Status: rpt.Survived},
	}}

	issue := codeClimateLineIssue(line, "mod/")

	assert.Equal(t, "gooze/survived", issue.CheckName)
	assert.Equal(t, "major", issue.Severity)
	assert.Equal(t, "mod/a.go", issue.Location.Path)
	assert.Equal(t, 7, issue.Location.Lines.Begin)
	assert.Equal(t, fingerprintOf([]string{"a-9", "z-1"}), issue.Fingerprint, "same fingerprint as SARIF")
}

func TestCodeClimateIssue_CheckWithoutASurvivor(t *testing.T) {
	tests := []struct {
		status rpt.TestStatus
		want   string
	}{
		{status: rpt.NotCovered, want: "gooze/not-covered"},
		{status: rpt.Timeout, want: "gooze/timeout"},
		{status: rpt.Skipped, want: "gooze/skipped"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			line := survivorLine{lineKey: lineKey{Path: "a.go", Line: 1}, Survivors: []Survivor{
				{MutationID: "s-1", Mutagen: "boolean", Status: rpt.Skipped},
				{MutationID: "s-2", Mutagen: "boolean", Status: tt.status},
			}}

			issue := codeClimateLineIssue(line, "")
			assert.Equal(t, tt.want, issue.CheckName)
			assert.Equal(t, "minor", issue.Severity)
		})
	}
}

func TestCodeClimateIssue_LineBegin(t *testing.T) {
	tests := []struct {
		line int
		want int
	}{
		{line: 2, want: 2},
		{line: 1, want: 1},
		{line: 0, want: 1},
		{line: -1, want: 1},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.line), func(t *testing.T) {
			line := survivorLine{lineKey: lineKey{Path: "a.go", Line: tt.line}, Survivors: []Survivor{{MutationID: "s-1", Status: rpt.Skipped}}}

			assert.Equal(t, tt.want, codeClimateLineIssue(line, "").Location.Lines.Begin)
		})
	}
}

func TestWriteCodeClimate_ReportsWriteErrors(t *testing.T) {
	err := writeCodeClimate(failingWriter{}, nil, "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "disk full")
}
