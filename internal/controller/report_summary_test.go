package controller

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"gooze.dev/pkg/gooze/internal/domain"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

func sampleReportSummary() domain.ReportSummary {
	return domain.ReportSummary{
		Score: domain.MutationScore{Summary: rpt.Summary{Total: 17, Killed: 14, Survived: 2, NotCovered: 1}},
		Files: []domain.FileSummary{
			{Path: "calc.go", Total: 16, Killed: 14, Survived: 2},
			{Path: "main.go", Total: 1, NotCovered: 1},
		},
		Survivors: []domain.Survivor{
			{Path: "calc.go", Line: 10, Mutagen: "comparison", Status: rpt.Survived, Original: "if n < lo {", Mutated: "if n <= lo {"},
			{Path: "main.go", Line: 4, Mutagen: "statement", Status: rpt.NotCovered, Original: "run()"},
		},
	}
}

func TestRenderReportSummary(t *testing.T) {
	output := renderReportSummary(sampleReportSummary())

	for _, want := range []string{
		"PATH",
		"NOT COVERED",
		"calc.go",
		"87.50%",
		"TOTAL FILES 2",
		"82.35%",
		"Survivors (2):\n" +
			"calc.go:10: survived (comparison): if n < lo { -> if n <= lo {\n" +
			"main.go:4: not_covered (statement): run() -> (line removed)\n",
		"\nMutation score: 82.35% (14 killed, 2 survived, 1 not_covered)\n",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q\noutput:\n%s", want, output)
		}
	}
}

func TestRenderReportSummary_NoSurvivors(t *testing.T) {
	output := renderReportSummary(domain.ReportSummary{
		Score: domain.MutationScore{Summary: rpt.Summary{Total: 1, Killed: 1}},
		Files: []domain.FileSummary{{Path: "a.go", Total: 1, Killed: 1}},
	})

	if !strings.Contains(output, "No survivors.\n") {
		t.Fatalf("output missing no-survivors line\noutput:\n%s", output)
	}
}

func TestFormatSurvivor(t *testing.T) {
	cases := []struct {
		survivor domain.Survivor
		want     string
	}{
		{
			survivor: domain.Survivor{Path: "a.go", Line: 3, Mutagen: "numbers", Status: rpt.Timeout, Original: "x := 1", Mutated: "x := 0"},
			want:     "a.go:3: timeout (numbers): x := 1 -> x := 0",
		},
		{
			survivor: domain.Survivor{Path: "a.go", Line: 3, Mutagen: "statement", Status: rpt.Survived, Original: "f()"},
			want:     "a.go:3: survived (statement): f() -> (line removed)",
		},
		{
			survivor: domain.Survivor{Path: "a.go", Mutagen: "boolean", Status: rpt.Skipped},
			want:     "a.go: skipped (boolean)",
		},
	}

	for _, tc := range cases {
		if got := formatSurvivor(tc.survivor); got != tc.want {
			t.Fatalf("formatSurvivor() = %q, want %q", got, tc.want)
		}
	}
}

func TestDisplayReportSummary_PrintsPlainText(t *testing.T) {
	want := renderReportSummary(sampleReportSummary())

	var simpleOut bytes.Buffer

	cmd := &cobra.Command{}
	cmd.SetOut(&simpleOut)
	NewSimpleUI(cmd).DisplayReportSummary(context.Background(), sampleReportSummary())

	var tuiOut bytes.Buffer

	NewTUI(&tuiOut).DisplayReportSummary(context.Background(), sampleReportSummary())

	if simpleOut.String() != want || tuiOut.String() != want {
		t.Fatalf("simple UI and TUI should print the rendered summary\nsimple:\n%s\ntui:\n%s", simpleOut.String(), tuiOut.String())
	}
}
