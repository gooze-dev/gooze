package controller

import (
	"testing"

	"gooze.dev/pkg/gooze/internal/domain"
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
