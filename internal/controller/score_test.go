package controller

import (
	"path/filepath"
	"strings"
	"testing"

	"gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

func TestFormatMutationScore(t *testing.T) {
	cases := []struct {
		name  string
		score domain.MutationScore
		want  string
	}{
		{
			name:  "no mutations",
			score: domain.MutationScore{},
			want:  "100.00% (no mutations)",
		},
		{
			name:  "reports directory",
			score: domain.MutationScore{Summary: rpt.Summary{Total: 4, Killed: 3, Survived: 1}},
			want:  "75.00% (3 killed, 1 survived)",
		},
		{
			name: "fully cached run",
			score: domain.MutationScore{
				Summary: rpt.Summary{Total: 16, Killed: 14, Survived: 2},
				Cached:  16,
			},
			want: "87.50% (14 killed, 2 survived; 16 cached, 0 tested)",
		},
		{
			name:  "uncached run",
			score: domain.MutationScore{Summary: rpt.Summary{Total: 1, Killed: 1}, Tested: 1},
			want:  "100.00% (1 killed, 0 survived; 0 cached, 1 tested)",
		},
		{
			name:  "one cached result",
			score: domain.MutationScore{Summary: rpt.Summary{Total: 1, Survived: 1}, Cached: 1},
			want:  "0.00% (0 killed, 1 survived; 1 cached, 0 tested)",
		},
		{
			name: "other statuses when present",
			score: domain.MutationScore{
				Summary: rpt.Summary{Total: 8, Killed: 2, Survived: 1, NotCovered: 1, Timeout: 1, Skipped: 1, Error: 2},
				Cached:  3,
				Tested:  5,
			},
			want: "33.33% (2 killed, 1 survived, 1 not_covered, 1 timeout, 1 skipped, 2 error; 3 cached, 5 tested)",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatMutationScore(tc.score); got != tc.want {
				t.Fatalf("formatMutationScore() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCachedEstimateHint(t *testing.T) {
	if got := cachedEstimateHint(domain.Estimation{Total: 3}); got != "" {
		t.Fatalf("cachedEstimateHint() without cached files = %q, want empty", got)
	}

	want := "2 of 3 mutations are in cached files and will not be re-tested; use --no-cache to re-test everything."
	if got := cachedEstimateHint(domain.Estimation{Total: 3, Cached: 2}); got != want {
		t.Fatalf("cachedEstimateHint() = %q, want %q", got, want)
	}
}

func TestDiffEstimateLine(t *testing.T) {
	cases := []struct {
		name string
		diff *domain.DiffInfo
		want string
	}{
		{name: "not diff mode", diff: nil, want: ""},
		{
			name: "one file, long base shortened",
			diff: &domain.DiffInfo{Ref: "origin/main", Base: "1a2b3c4d5e6f7a8b", Files: 1},
			want: "Diff mode: 1 file changed since origin/main (merge base 1a2b3c4); counts cover changed lines only.",
		},
		{
			name: "several files, short base kept",
			diff: &domain.DiffInfo{Ref: "HEAD~1", Base: "abc", Files: 3},
			want: "Diff mode: 3 files changed since HEAD~1 (merge base abc); counts cover changed lines only.",
		},
		{
			name: "no changed files",
			diff: &domain.DiffInfo{Ref: "main", Base: "1a2b3c4d", Files: 0},
			want: "Diff mode: no changed files since main (merge base 1a2b3c4).",
		},
	}

	for _, tc := range cases {
		if got := diffEstimateLine(domain.Estimation{Diff: tc.diff}); got != tc.want {
			t.Errorf("%s: diffEstimateLine() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestSelectionLine(t *testing.T) {
	cases := []struct {
		name       string
		estimation domain.Estimation
		want       string
	}{
		{name: "no filters", estimation: domain.Estimation{}, want: ""},
		{
			name:       "run",
			estimation: domain.Estimation{Run: "^InRange$"},
			want:       "Only functions matching ^InRange$.",
		},
		{
			name:       "mutagens",
			estimation: domain.Estimation{Mutagens: []string{"comparison", "arithmetic"}},
			want:       "Only mutagens comparison, arithmetic.",
		},
		{
			name:       "both",
			estimation: domain.Estimation{Run: "Next$", Mutagens: []string{"comparison"}},
			want:       "Only functions matching Next$, mutagens comparison.",
		},
	}

	for _, tc := range cases {
		if got := selectionLine(tc.estimation); got != tc.want {
			t.Errorf("%s: selectionLine() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestFormatMutationScore_NarrowedRun(t *testing.T) {
	score := domain.MutationScore{
		Summary:   rpt.Summary{Total: 4, Killed: 2, Survived: 2},
		Tested:    4,
		Selection: "functions matching ^Add$",
	}

	want := "50.00% (2 killed, 2 survived; 0 cached, 4 tested; functions matching ^Add$)"
	if got := formatMutationScore(score); got != want {
		t.Errorf("formatMutationScore() = %q, want %q", got, want)
	}

	// The CI gate's sed reads the score from the start of the line.
	if !strings.HasPrefix(want, "50.00%") {
		t.Fatal("score must lead the line")
	}

	empty := domain.MutationScore{Selection: "functions matching ^Nope$"}
	if got := formatMutationScore(empty); got != "100.00% (no mutations; functions matching ^Nope$)" {
		t.Errorf("formatMutationScore(no mutations) = %q", got)
	}
}

func TestRunReportsHint(t *testing.T) {
	if got := runReportsHint(domain.MutationScore{}); got != "" {
		t.Errorf("full run hint = %q, want none", got)
	}

	got := runReportsHint(domain.MutationScore{ReportsDir: m.Path(filepath.Join(".gooze-reports", "run"))})
	want := "Reports of this run are in " + filepath.Join(".gooze-reports", "run") + "; add them to the main reports with gooze report merge."

	if got != want {
		t.Errorf("runReportsHint() = %q, want %q", got, want)
	}
}
