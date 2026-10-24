package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// file is a FileSummary with killed of scored mutations killed, plus errors
// mutations that don't compile.
func file(path string, killed, scored, errors int) domain.FileSummary {
	return domain.FileSummary{Path: path, Summary: rpt.Summary{
		Total: scored + errors, Killed: killed, Survived: scored - killed, Error: errors,
	}}
}

// scoreOf is the MutationScore of files, as a run or view computes it.
func scoreOf(files ...domain.FileSummary) domain.MutationScore {
	score := domain.MutationScore{Files: files}
	for _, f := range files {
		score.Merge(f.Summary)
	}

	return score
}

func TestThresholds_Set(t *testing.T) {
	assert.False(t, domain.Thresholds{}.Set())
	assert.True(t, domain.Thresholds{Total: 1}.Set())
	assert.True(t, domain.Thresholds{Average: 0.5}.Set())
	assert.True(t, domain.Thresholds{File: 100}.Set())
	assert.True(t, domain.Thresholds{Total: 0.5}.Set())
	assert.True(t, domain.Thresholds{File: 0.5}.Set())
	assert.True(t, domain.Thresholds{MaxSurvivors: 1}.Set())
}

func TestThresholds_Check(t *testing.T) {
	// 11 of 13 is 84.615...%, shown and compared as 84.62%.
	calc := scoreOf(file("calc.go", 11, 13, 0))
	// 85.71% total; the files score 100%, 50% and 100%: average 83.33%.
	project := scoreOf(file("a.go", 4, 4, 0), file("b.go", 1, 2, 0), file("c.go", 1, 1, 0))

	cases := map[string]struct {
		score domain.MutationScore
		t     domain.Thresholds
		want  []string
	}{
		"none set":            {calc, domain.Thresholds{}, nil},
		"total met":           {calc, domain.Thresholds{Key: "threshold", Total: 84}, nil},
		"total equal":         {calc, domain.Thresholds{Key: "threshold", Total: 84.62}, nil},
		"total below":         {calc, domain.Thresholds{Key: "threshold", Total: 84.63}, []string{"mutation score 84.62% is below threshold.total 84.63%"}},
		"average met":         {project, domain.Thresholds{Key: "threshold", Average: 83.33}, nil},
		"average below":       {project, domain.Thresholds{Key: "threshold", Average: 83.34}, []string{"average file score 83.33% is below threshold.average 83.34%"}},
		"file met":            {project, domain.Thresholds{Key: "threshold", File: 50}, nil},
		"file below":          {project, domain.Thresholds{Key: "threshold", File: 50.01}, []string{"b.go: score 50.00% is below threshold.file 50.01%"}},
		"every file below":    {project, domain.Thresholds{Key: "threshold", File: 100.0}, []string{"b.go: score 50.00% is below threshold.file 100%"}},
		"several, diff names": {project, domain.Thresholds{Key: "threshold.diff", Total: 90, Average: 90, File: 75}, []string{"mutation score 85.71% is below threshold.diff.total 90%", "average file score 83.33% is below threshold.diff.average 90%", "b.go: score 50.00% is below threshold.diff.file 75%"}},
		"files sorted": {scoreOf(file("z.go", 0, 1, 0), file("a.go", 0, 1, 0)), domain.Thresholds{Key: "threshold", File: 1},
			[]string{"a.go: score 0.00% is below threshold.file 1%", "z.go: score 0.00% is below threshold.file 1%"}},
		// A file of mutations that don't compile has no score, so it neither
		// fails threshold.file nor counts towards the average.
		"unscored file":                              {scoreOf(file("a.go", 1, 1, 0), file("b.go", 0, 0, 3)), domain.Thresholds{Key: "threshold", Average: 100, File: 100}, nil},
		"one file below the average":                 {calc, domain.Thresholds{Key: "threshold", Average: 85}, []string{"average file score 84.62% is below threshold.average 85%"}},
		"a fraction of a percent":                    {scoreOf(file("a.go", 0, 1, 0)), domain.Thresholds{Key: "threshold", Total: 0.5, Average: 0.5, File: 0.5}, []string{"mutation score 0.00% is below threshold.total 0.5%", "average file score 0.00% is below threshold.average 0.5%", "a.go: score 0.00% is below threshold.file 0.5%"}},
		"zero is off":                                {scoreOf(file("a.go", 0, 1, 0)), domain.Thresholds{Key: "threshold"}, nil},
		"an unscored file doesn't raise the average": {scoreOf(file("a.go", 1, 2, 0), file("b.go", 0, 0, 3)), domain.Thresholds{Key: "threshold", Average: 60}, []string{"average file score 50.00% is below threshold.average 60%"}},
		"no mutations":                               {scoreOf(), domain.Thresholds{Key: "threshold", Total: 100, Average: 100, File: 100}, nil},
		"nothing scored":                             {scoreOf(file("a.go", 0, 0, 2)), domain.Thresholds{Key: "threshold", Total: 100, Average: 100}, nil},
		// calc.go has 2 survivors.
		"max survivors met":         {calc, domain.Thresholds{Key: "threshold", MaxSurvivors: 2}, nil},
		"max survivors above":       {calc, domain.Thresholds{Key: "threshold", MaxSurvivors: 1}, []string{"2 surviving mutants are above threshold.max_survivors 1"}},
		"max survivors, diff run":   {calc, domain.Thresholds{Key: "threshold.diff", MaxSurvivors: 1}, []string{"2 surviving mutants are above threshold.max_survivors 1"}},
		"max survivors of 0 is off": {project, domain.Thresholds{Key: "threshold", Total: 90, MaxSurvivors: 0, File: 75}, []string{"mutation score 85.71% is below threshold.total 90%", "b.go: score 50.00% is below threshold.file 75%"}},
		"survivors and scores":      {scoreOf(file("a.go", 1, 4, 0)), domain.Thresholds{Key: "threshold", Total: 50, MaxSurvivors: 2}, []string{"mutation score 25.00% is below threshold.total 50%", "3 surviving mutants are above threshold.max_survivors 2"}},
		"only survived counts":      {domain.MutationScore{Summary: rpt.Summary{Total: 3, Survived: 1, NotCovered: 1, Timeout: 1}}, domain.Thresholds{Key: "threshold", MaxSurvivors: 1}, nil},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var got []string
			for _, b := range tc.t.Check(tc.score) {
				got = append(got, b.String())
			}

			assert.Equal(t, tc.want, got)
		})
	}
}

func TestThresholds_Err(t *testing.T) {
	assert.NoError(t, domain.Thresholds{Key: "threshold", Total: 50}.Err(scoreOf(file("a.go", 1, 1, 0))))

	err := domain.Thresholds{Key: "threshold", Total: 90, File: 90}.Err(scoreOf(file("a.go", 1, 2, 0)))
	require.EqualError(t, err, "mutation score 50.00% is below threshold.total 90%\na.go: score 50.00% is below threshold.file 90%")
	assert.Equal(t, m.ExitBelowThreshold, exitCodeOf(err))

	var below *domain.ThresholdError
	require.ErrorAs(t, err, &below)
	assert.Len(t, below.Breaches, 2)
}
