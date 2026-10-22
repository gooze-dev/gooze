package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

func TestEscapeAnnotation(t *testing.T) {
	assert.Equal(t, "50%25 done%0D%0Anext: a, b", escapeAnnotationData("50% done\r\nnext: a, b"))
	assert.Equal(t, "a%3Ab%2Cc%25d%0Ae%0Df", escapeAnnotationProperty("a:b,c%d\ne\rf"))
}

func TestGitHubAnnotation(t *testing.T) {
	tests := []struct {
		name     string
		survivor Survivor
		want     string
	}{
		{
			name:     "survived",
			survivor: Survivor{Path: "a.go", Line: 3, Mutagen: "numbers", Status: rpt.Survived, Original: "x := 1", Mutated: "x := 0"},
			want:     "::warning file=a.go,line=3,title=Surviving mutant (numbers)::Tests still pass with `x := 1` changed to `x := 0`",
		},
		{
			name:     "removed line",
			survivor: Survivor{Path: "a.go", Line: 3, Mutagen: "statement", Status: rpt.Survived, Original: "f()"},
			want:     "::warning file=a.go,line=3,title=Surviving mutant (statement)::Tests still pass with `f()` removed",
		},
		{
			name:     "timeout",
			survivor: Survivor{Path: "a.go", Line: 4, Mutagen: "loop", Status: rpt.Timeout, Original: "i++", Mutated: "i--"},
			want:     "::notice file=a.go,line=4,title=Timed-out mutant (loop)::Tests time out with `i++` changed to `i--`",
		},
		{
			name:     "skipped without a diff",
			survivor: Survivor{Path: "a.go", Mutagen: "boolean", Status: rpt.Skipped},
			want:     "::notice file=a.go,title=Skipped mutant (boolean)::Not tested: this boolean mutation",
		},
		{
			name:     "a line below 1 annotates the file",
			survivor: Survivor{Path: "a.go", Line: -1, Mutagen: "boolean", Status: rpt.Skipped},
			want:     "::notice file=a.go,title=Skipped mutant (boolean)::Not tested: this boolean mutation",
		},
		{
			name:     "escapes the path and the message",
			survivor: Survivor{Path: "dir,x/a:b.go", Line: 1, Mutagen: "arithmetic", Status: rpt.Survived, Original: "p := 100 % n", Mutated: "p := 100 * n"},
			want:     "::warning file=dir%2Cx/a%3Ab.go,line=1,title=Surviving mutant (arithmetic)::Tests still pass with `p := 100 %25 n` changed to `p := 100 * n`",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, []string{tt.want}, gitHubAnnotations([]Survivor{tt.survivor}, ""))
		})
	}
}

func TestGitHubAnnotation_PathPrefix(t *testing.T) {
	survivor := Survivor{Path: "pkg/a.go", Line: 2, Mutagen: "comparison", Status: rpt.NotCovered, Original: "a < b", Mutated: "a <= b"}

	assert.Equal(t, []string{"::notice file=mod/pkg/a.go,line=2,title=Uncovered mutant (comparison)::No test covers `a < b` changed to `a <= b`"},
		gitHubAnnotations([]Survivor{survivor}, "mod/"))
}

func TestGitHubAnnotations_CombinesMutantsOnOneLine(t *testing.T) {
	survivors := []Survivor{
		{Path: "a.go", Line: 10, Mutagen: "arithmetic", Status: rpt.NotCovered, Original: "if n+1 < lo {", Mutated: "if n-1 < lo {", MutationID: "m2"},
		{Path: "a.go", Line: 10, Mutagen: "comparison", Status: rpt.Survived, Original: "if n+1 < lo {", Mutated: "if n+1 <= lo {", MutationID: "m1"},
		{Path: "a.go", Line: 12, Mutagen: "comparison", Status: rpt.Survived, Original: "if n > hi {", Mutated: "if n >= hi {", MutationID: "m3"},
		{Path: "a.go", Line: 12, Mutagen: "comparison", Status: rpt.Survived, Original: "if n > hi {", Mutated: "if n != hi {", MutationID: "m4"},
		{Path: "b.go", Line: 10, Mutagen: "boolean", Status: rpt.Timeout, Original: "ok := true", Mutated: "ok := false", MutationID: "m5"},
	}

	assert.Equal(t, []string{
		"::warning file=a.go,line=10,title=2 mutants (arithmetic%2C comparison)::" +
			"No test covers `if n+1 < lo {` changed to `if n-1 < lo {`%0ATests still pass with `if n+1 < lo {` changed to `if n+1 <= lo {`",
		"::warning file=a.go,line=12,title=2 surviving mutants (comparison)::" +
			"Tests still pass with `if n > hi {` changed to `if n >= hi {`%0ATests still pass with `if n > hi {` changed to `if n != hi {`",
		"::notice file=b.go,line=10,title=Timed-out mutant (boolean)::Tests time out with `ok := true` changed to `ok := false`",
	}, gitHubAnnotations(survivors, ""))
}

func TestGitHubAnnotations_GroupsOnlyTheSameFileAndLine(t *testing.T) {
	survivors := []Survivor{
		{Path: "a.go", Line: 5, Mutagen: "comparison", Status: rpt.Survived, Original: "a < b", Mutated: "a <= b"},
		{Path: "b.go", Line: 5, Mutagen: "comparison", Status: rpt.Survived, Original: "c < d", Mutated: "c <= d"},
		{Path: "b.go", Line: 5, Mutagen: "comparison", Status: rpt.Survived, Original: "c < d", Mutated: "c != d"},
	}

	annotations := gitHubAnnotations(survivors, "")

	require.Len(t, annotations, 2)
	assert.Contains(t, annotations[0], "file=a.go,line=5,title=Surviving mutant (comparison)::")
	assert.Contains(t, annotations[1], "file=b.go,line=5,title=2 surviving mutants (comparison)::")
}

func TestGitHubAnnotations_MixedStatusesWhenTheFirstSurvived(t *testing.T) {
	survivors := []Survivor{
		{Path: "a.go", Line: 7, Mutagen: "arithmetic", Status: rpt.Survived, Original: "a + b", Mutated: "a - b"},
		{Path: "a.go", Line: 7, Mutagen: "comparison", Status: rpt.NotCovered, Original: "a + b", Mutated: "a * b"},
	}

	assert.Equal(t, []string{
		"::warning file=a.go,line=7,title=2 mutants (arithmetic%2C comparison)::" +
			"Tests still pass with `a + b` changed to `a - b`%0ANo test covers `a + b` changed to `a * b`",
	}, gitHubAnnotations(survivors, ""))
}
