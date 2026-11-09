package controller

import (
	"bytes"
	"context"
	"reflect"
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

func TestRenderFileSummaryTable_RowsAddUpToMutations(t *testing.T) {
	output := renderFileSummaryTable(domain.ReportSummary{
		Score: domain.MutationScore{Summary: rpt.Summary{Total: 10, Killed: 4, Survived: 3, NotCovered: 1, Timeout: 1, Error: 1}},
		Files: []domain.FileSummary{{Path: "a.go", Summary: rpt.Summary{Total: 10, Killed: 4, Survived: 3, NotCovered: 1, Timeout: 1, Error: 1}}},
	})

	var cells [][]string

	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "│") {
			row := strings.Split(line, "│")
			for i := range row {
				row[i] = strings.TrimSpace(row[i])
			}

			cells = append(cells, row)
		}
	}

	want := [][]string{
		{"PATH", "MUTATIONS", "KILLED", "SURVIVED", "NOT COVERED", "TIMEOUT", "ERROR", "SCORE"},
		{"a.go", "10", "4", "3", "1", "1", "1", "44.44%"},
		{"TOTAL FILES 1", "10", "4", "3", "1", "1", "1", "44.44%"},
	}

	if !reflect.DeepEqual(cells, want) {
		t.Fatalf("cells = %q, want %q\noutput:\n%s", cells, want, output)
	}
}

func TestRenderReportSummary_BuildErrors(t *testing.T) {
	summary := sampleReportSummary()
	summary.BuildErrors = []domain.Survivor{
		{Path: "calc.go", Line: 12, Mutagen: "arithmetic", Status: rpt.Error, Original: "a + b", Mutated: "a * b"},
	}

	output := renderReportSummary(summary)

	want := "\nBuild errors (1):\ncalc.go:12: error (arithmetic): a + b -> a * b\n\nMutation score: "
	if !strings.Contains(output, want) {
		t.Fatalf("output missing %q\noutput:\n%s", want, output)
	}

	if output := renderReportSummary(sampleReportSummary()); strings.Contains(output, "Build errors") {
		t.Fatalf("no build errors, no section\noutput:\n%s", output)
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
			survivor: domain.Survivor{Path: "a.go", Line: 1, Mutagen: "boolean", Status: rpt.Survived, Original: "var x = true", Mutated: "var x = false"},
			want:     "a.go:1: survived (boolean): var x = true -> var x = false",
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

func TestRenderReportSummary_Hint(t *testing.T) {
	if output := renderReportSummary(domain.ReportSummary{Hint: "run the merge"}); !strings.HasSuffix(output, "\nrun the merge\n") {
		t.Fatalf("output = %q", output)
	}

	if output := renderReportSummary(domain.ReportSummary{}); strings.HasSuffix(output, "\n\n") {
		t.Fatalf("no hint leaves no blank line: %q", output)
	}
}

func survivorLines(survivors ...domain.Survivor) domain.ReportSummary {
	summary := domain.ReportSummary{Survivors: survivors}

	for _, survivor := range survivors {
		lines := summary.SurvivorLines
		if n := len(lines); n > 0 && lines[n-1].Path == survivor.Path && lines[n-1].Line == survivor.Line {
			lines[n-1].Survivors = append(lines[n-1].Survivors, survivor)
			continue
		}

		line := domain.SurvivorLine{Survivors: []domain.Survivor{survivor}}
		line.Path, line.Line = survivor.Path, survivor.Line
		summary.SurvivorLines = append(lines, line)
	}

	return summary
}

func TestRenderReportSummary_ByLine(t *testing.T) {
	const original = "if len(apiKeys) > 0 {"

	mutant := func(mutagen, mutated string) domain.Survivor {
		return domain.Survivor{Path: "runner/config.go", Line: 56, Mutagen: mutagen, Status: rpt.Survived, Original: original, Mutated: mutated}
	}

	output := renderReportSummary(survivorLines(
		mutant("comparison", "if len(apiKeys) >= 0 {"),
		mutant("comparison", "if len(apiKeys) == 0 {"),
		mutant("comparison", "if len(apiKeys) <= 0 {"),
		mutant("comparison", "if len(apiKeys) < 0 {"),
		mutant("numbers", "if len(apiKeys) > 1 {"),
		domain.Survivor{Path: "z.go", Line: 3, Mutagen: "boolean", Status: rpt.Survived, Original: "a := true", Mutated: "a := false"},
	))

	want := "Survivors (6 on 2 lines):\n" +
		"runner/config.go:56: 5 survived (comparison ×4, numbers): if len(apiKeys) > 0 { -> >= 0, == 0, <= 0, < 0, > 1\n" +
		"z.go:3: survived (boolean): a := true -> a := false\n\n"
	if !strings.Contains(output, want) {
		t.Fatalf("output missing %q\noutput:\n%s", want, output)
	}
}

func TestRenderReportSummary_ByLineOneLineIsSingular(t *testing.T) {
	output := renderReportSummary(survivorLines(domain.Survivor{Path: "a.go", Line: 1, Mutagen: "boolean", Status: rpt.Survived}))

	if !strings.Contains(output, "Survivors (1 on 1 line):\n") {
		t.Fatalf("output:\n%s", output)
	}
}

func TestRenderReportSummary_BuildErrorsStayFlatByLine(t *testing.T) {
	summary := survivorLines(domain.Survivor{Path: "a.go", Line: 1, Mutagen: "boolean", Status: rpt.Survived})
	failure := domain.Survivor{Path: "b.go", Line: 2, Mutagen: "numbers", Status: rpt.Error, Original: "x", Mutated: "y"}
	summary.BuildErrors = []domain.Survivor{failure, failure}

	want := "Build errors (2):\nb.go:2: error (numbers): x -> y\nb.go:2: error (numbers): x -> y\n"
	if output := renderReportSummary(summary); !strings.Contains(output, want) {
		t.Fatalf("output missing %q\noutput:\n%s", want, output)
	}
}

func TestFormatSurvivorLine(t *testing.T) {
	survivor := func(status rpt.TestStatus, mutagen, mutated string) domain.Survivor {
		return domain.Survivor{Path: "a.go", Line: 7, Mutagen: mutagen, Status: status, Original: "x := a + b", Mutated: mutated}
	}

	for name, tt := range map[string]struct {
		survivors []domain.Survivor
		want      string
	}{
		"one mutant reads as in the flat list": {
			survivors: []domain.Survivor{survivor(rpt.Survived, "arithmetic", "x := a - b")},
			want:      "a.go:7: survived (arithmetic): x := a + b -> x := a - b",
		},
		"mixed statuses in order": {
			survivors: []domain.Survivor{
				survivor(rpt.Survived, "arithmetic", "x := a - b"),
				survivor(rpt.NotCovered, "arithmetic", "x := a * b"),
				survivor(rpt.Survived, "arithmetic", "x := a / b"),
			},
			want: "a.go:7: 2 survived, 1 not_covered (arithmetic ×3): x := a + b -> -, *, /",
		},
		"a removed line among the variants": {
			survivors: []domain.Survivor{
				survivor(rpt.Survived, "arithmetic", "x := a - b"),
				survivor(rpt.Survived, "arithmetic", "x := a * b"),
				survivor(rpt.Survived, "statement", ""),
			},
			want: "a.go:7: 3 survived (arithmetic ×2, statement): x := a + b -> -, *, (line removed)",
		},
		"no location and no change": {
			survivors: []domain.Survivor{
				{Path: "a.go", Mutagen: "boolean", Status: rpt.Survived},
				{Path: "a.go", Mutagen: "boolean", Status: rpt.Survived},
			},
			want: "a.go: 2 survived (boolean ×2)",
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := formatSurvivorLine(survivorLines(tt.survivors...).SurvivorLines[0]); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFoldChanges(t *testing.T) {
	for name, tt := range map[string]struct {
		original string
		mutated  []string
		want     []string
	}{
		"common ends go":          {"a b c", []string{"a x c", "a y c"}, []string{"x", "y"}},
		"only tokens in common":   {"a b c", []string{"a x y", "a x c"}, []string{"x y", "x c"}},
		"the first token differs": {"a b", []string{"x b", "y b"}, []string{"x", "y"}},
		"the last token differs":  {"a b", []string{"a x", "a y"}, []string{"x", "y"}},
		"a lone token stays":      {"a", []string{"x", "y"}, []string{"x", "y"}},
		"a shorter variant keeps a token": {
			"a  b", []string{"a b", "a x b"}, []string{"b", "x b"},
		},
		"prefix and suffix don't overlap": {
			"a a", []string{"a a a", "a a a a"}, []string{"a a", "a a a"},
		},
		"a suffix among lines of different lengths": {"x c", []string{"y z c", "q c"}, []string{"y z", "q"}},
		"a token sorting before the original's":     {"z c", []string{"a c", "b c"}, []string{"a", "b"}},
		"every line removed":                        {"a b", []string{"", ""}, []string{lineRemoved, lineRemoved}},
		"inner spaces stay":                         {"a  b c", []string{"x  b d", "y  b d"}, []string{"x  b d", "y  b d"}},
	} {
		t.Run(name, func(t *testing.T) {
			if got := foldChanges(tt.original, tt.mutated); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenderReportSummary_Brief(t *testing.T) {
	summary := sampleReportSummary()
	summary.Brief = true
	summary.BuildErrors = []domain.Survivor{{Path: "calc.go", Line: 3, Mutagen: "numbers", Status: rpt.Error}}

	output := renderReportSummary(summary)

	for _, want := range []string{"TOTAL FILES 2", "\nMutation score: 82.35% (14 killed, 2 survived, 1 not_covered)\n"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q\noutput:\n%s", want, output)
		}
	}

	for _, unwanted := range []string{"Survivors", "No survivors", "Build errors", "calc.go:10"} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("a brief summary lists %q\noutput:\n%s", unwanted, output)
		}
	}
}

func TestRenderReportSummary_HiddenFiles(t *testing.T) {
	summary := sampleReportSummary()
	summary.Files = summary.Files[:1]
	summary.HiddenFiles = 1

	output := renderReportSummary(summary)

	for _, want := range []string{"TOTAL FILES 2", "The 1 of 2 files with the most survivors.\n"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q\noutput:\n%s", want, output)
		}
	}

	summary.HiddenFiles = 0
	if strings.Contains(renderReportSummary(summary), "most survivors") {
		t.Fatal("a full table names no top files")
	}
}

func TestRenderReportSummary_Sections(t *testing.T) {
	previous := sectionClock
	sectionClock = func() int64 { return 1700000000 }

	t.Cleanup(func() { sectionClock = previous })

	summary := sampleReportSummary()
	summary.Sections = true
	summary.BuildErrors = []domain.Survivor{{Path: "calc.go", Line: 3, Mutagen: "numbers", Status: rpt.Error}}

	output := renderReportSummary(summary)

	for _, want := range []string{
		"\x1b[0Ksection_start:1700000000:gooze_files\r\x1b[0KMutation scores per file\n PATH",
		"TOTAL FILES 2",
		"\x1b[0Ksection_end:1700000000:gooze_files\r\x1b[0K\n",
		"\x1b[0Ksection_start:1700000000:gooze_survivors[collapsed=true]\r\x1b[0KSurvivors (2):\ncalc.go:10:",
		"\x1b[0Ksection_end:1700000000:gooze_survivors\r\x1b[0K\n",
		"\x1b[0Ksection_start:1700000000:gooze_build_errors[collapsed=true]\r\x1b[0KBuild errors (1):\ncalc.go:3:",
		"\x1b[0Ksection_end:1700000000:gooze_build_errors\r\x1b[0K\n",
		"\nMutation score: 82.35%",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q\noutput:\n%q", want, output)
		}
	}

	if strings.Contains(renderReportSummary(sampleReportSummary()), "section_start") {
		t.Fatal("sections only when asked for")
	}
}

func TestRenderReportSummary_Scope(t *testing.T) {
	summary := sampleReportSummary()
	if output := renderReportSummary(summary); !strings.HasPrefix(output, " PATH") {
		t.Fatalf("a summary without a scope starts with its table\noutput:\n%q", output)
	}

	summary.Scope = domain.ReportScope{Name: "diff", Title: "Changed lines since main"}

	output := renderReportSummary(summary)
	if !strings.HasPrefix(output, "\nChanged lines since main\n PATH") {
		t.Fatalf("a scope's title heads its table\noutput:\n%s", output)
	}

	previous := sectionClock
	sectionClock = func() int64 { return 1700000000 }

	t.Cleanup(func() { sectionClock = previous })

	summary.Sections = true
	output = renderReportSummary(summary)

	for _, want := range []string{
		"\x1b[0Ksection_start:1700000000:gooze_diff_files\r\x1b[0KMutation scores per file: Changed lines since main\n PATH",
		"\x1b[0Ksection_end:1700000000:gooze_diff_files\r\x1b[0K\n",
		"\x1b[0Ksection_start:1700000000:gooze_diff_survivors[collapsed=true]\r\x1b[0KSurvivors (2):\n",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q\noutput:\n%q", want, output)
		}
	}

	if strings.Contains(output, "\nChanged lines since main\n") {
		t.Fatalf("in a CI log the section title names the scope, without a heading line\noutput:\n%q", output)
	}
}
