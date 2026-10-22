package domain

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

func TestSARIFLineResult_MostSevereMutantSetsTheRule(t *testing.T) {
	line := survivorLine{lineKey: lineKey{Path: "a.go", Line: 7}, Survivors: []Survivor{
		{MutationID: "z-1", Mutagen: "boolean", Status: rpt.Timeout},
		{MutationID: "a-9", Mutagen: "arithmetic", Status: rpt.Survived},
		{MutationID: "m-5", Mutagen: "boolean", Status: rpt.NotCovered},
	}}

	result := sarifLineResult(line, "mod/")

	assert.Equal(t, "gooze/survived", result.RuleID)
	assert.Equal(t, "warning", result.Level)
	assert.Equal(t, "mod/a.go", result.Locations[0].PhysicalLocation.ArtifactLocation.URI)
	assert.Equal(t, &sarifRegion{StartLine: 7}, result.Locations[0].PhysicalLocation.Region)
	assert.Equal(t, []string{"a-9", "m-5", "z-1"}, result.Properties.MutationIDs)
	assert.Equal(t, []string{"boolean", "arithmetic"}, result.Properties.Mutagens)
}

func TestSARIFLineResult_RuleWithoutASurvivor(t *testing.T) {
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

			result := sarifLineResult(line, "")
			assert.Equal(t, tt.want, result.RuleID)
			assert.Equal(t, "note", result.Level)
		})
	}
}

func TestSARIFLineResult_Region(t *testing.T) {
	tests := []struct {
		line int
		want *sarifRegion
	}{
		{line: 1, want: &sarifRegion{StartLine: 1}},
		{line: 0},
		{line: -1},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.line), func(t *testing.T) {
			line := survivorLine{lineKey: lineKey{Path: "a.go", Line: tt.line}, Survivors: []Survivor{{MutationID: "s-1", Status: rpt.Skipped}}}

			assert.Equal(t, tt.want, sarifLineResult(line, "").Locations[0].PhysicalLocation.Region)
		})
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestWriteSARIF_ReportsWriteErrors(t *testing.T) {
	err := writeSARIF(failingWriter{}, nil, "", "v1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "disk full")
}
