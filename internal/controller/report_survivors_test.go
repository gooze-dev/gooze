package controller

import (
	"strings"
	"testing"

	"gooze.dev/pkg/gooze/internal/domain"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// cappedSurvivors has survivors on three lines of a.go, two of them on line
// 3, and on one line of b.go.
func cappedSurvivors() []domain.Survivor {
	survivor := func(path string, line int, mutated string) domain.Survivor {
		return domain.Survivor{Path: path, Line: line, Mutagen: "numbers", Status: rpt.Survived, Original: "n := 1", Mutated: mutated}
	}

	return []domain.Survivor{
		survivor("a.go", 3, "n := 0"), survivor("a.go", 3, "n := 2"), survivor("a.go", 9, "n := 0"), survivor("a.go", 12, "n := 0"),
		survivor("b.go", 5, "n := 0"),
	}
}

func TestWriteSurvivors_MaxPerFile(t *testing.T) {
	const (
		a3  = "a.go:3: survived (numbers): n := 1 -> n := 0\na.go:3: survived (numbers): n := 1 -> n := 2\n"
		a9  = "a.go:9: survived (numbers): n := 1 -> n := 0\n"
		a12 = "a.go:12: survived (numbers): n := 1 -> n := 0\n"
		b5  = "b.go:5: survived (numbers): n := 1 -> n := 0\n"
	)

	for name, tt := range map[string]struct {
		limit int
		want  string
	}{
		"no cap":    {limit: 0, want: "Survivors (5):\n" + a3 + a9 + a12 + b5},
		"one line":  {limit: 1, want: "Survivors (5):\n" + a3 + "a.go: and 2 more lines\n" + b5},
		"two lines": {limit: 2, want: "Survivors (5):\n" + a3 + a9 + "a.go: and 1 more line\n" + b5},
		"all three": {limit: 3, want: "Survivors (5):\n" + a3 + a9 + a12 + b5},
	} {
		t.Run(name, func(t *testing.T) {
			var out strings.Builder

			writeSurvivors(&out, domain.ReportSummary{Survivors: cappedSurvivors(), MaxPerFile: tt.limit}, logSection{out: &out})

			if out.String() != tt.want {
				t.Fatalf("got:\n%s\nwant:\n%s", out.String(), tt.want)
			}
		})
	}
}

func TestWriteSurvivors_MaxPerFileByLine(t *testing.T) {
	summary := survivorLines(cappedSurvivors()...)
	summary.MaxPerFile = 2

	var out strings.Builder

	writeSurvivors(&out, summary, logSection{out: &out})

	want := "Survivors (5 on 4 lines):\n" +
		"a.go:3: 2 survived (numbers ×2): n := 1 -> 0, 2\n" +
		"a.go:9: survived (numbers): n := 1 -> n := 0\n" +
		"a.go: and 1 more line\n" +
		"b.go:5: survived (numbers): n := 1 -> n := 0\n"
	if out.String() != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out.String(), want)
	}
}
