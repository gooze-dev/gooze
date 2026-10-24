package domain

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	m "gooze.dev/pkg/gooze/internal/model"
)

// Thresholds are the minimum mutation scores, in percent, that a run, view or
// merge must reach, and the most surviving mutants it may have; 0 turns one
// off. Key names the config object of the scores ("threshold", or
// "threshold.diff" for --diff runs) in messages.
type Thresholds struct {
	Key string
	// Total is the minimum score of all mutations.
	Total float64
	// Average is the minimum of the mean of the per-file scores.
	Average float64
	// File is the minimum score of every file.
	File float64
	// MaxSurvivors is the most surviving mutants allowed. It has no diff
	// copy: a --diff run counts the survivors on its changed lines.
	MaxSurvivors int
}

// maxSurvivorsKey is the config key of Thresholds.MaxSurvivors.
const maxSurvivorsKey = "threshold.max_survivors"

// Set reports whether any threshold is on.
func (t Thresholds) Set() bool {
	// Equivalent: thresholds are validated to 0..100, so > 0 is != 0.
	return t.Total > 0 || t.Average > 0 || t.File > 0 || t.MaxSurvivors > 0 //gooze:ignore comparison
}

// Breach is a score below one of the thresholds, or more survivors than
// threshold.max_survivors allows.
type Breach struct {
	// Key is the threshold's config key, e.g. "threshold.file".
	Key string
	// File is the file whose score is low, for threshold.file.
	File  string
	Score float64 // percent, as gooze prints it; the survivor count for threshold.max_survivors
	Min   float64 // percent; the maximum for threshold.max_survivors
}

func (b Breach) String() string {
	if b.Key == maxSurvivorsKey {
		return fmt.Sprintf("%d surviving mutants are above %s %d", int(b.Score), b.Key, int(b.Min))
	}

	what := "mutation score"

	switch {
	case b.File != "":
		what = b.File + ": score"
	case strings.HasSuffix(b.Key, ".average"):
		what = "average file score"
	}

	return fmt.Sprintf("%s %.2f%% is below %s %s%%", what, b.Score, b.Key, strconv.FormatFloat(b.Min, 'f', -1, 64))
}

// ThresholdError reports the scores below their thresholds, one per line.
type ThresholdError struct {
	Breaches []Breach
}

func (e *ThresholdError) Error() string {
	lines := make([]string, 0, len(e.Breaches))
	for _, b := range e.Breaches {
		lines = append(lines, b.String())
	}

	return strings.Join(lines, "\n")
}

// Err returns a ThresholdError, ending the command with ExitBelowThreshold,
// when score is below a threshold, and nil otherwise.
func (t Thresholds) Err(score MutationScore) error {
	breaches := t.Check(score)
	if len(breaches) == 0 {
		return nil
	}

	return WithExitCode(m.ExitBelowThreshold, &ThresholdError{Breaches: breaches})
}

// Check returns the scores below their thresholds: the total, the survivors
// above threshold.max_survivors, the average of the file scores, then each low
// file by path. Scores are compared as gooze
// prints them, rounded to two decimals. A threshold of 0 is off: no score is
// below it. Files with no scored mutations (none, or none that compile) have
// no score: they never fail threshold.file and don't count towards the
// average, and a set without scored mutations scores 100%, which meets every
// threshold.
func (t Thresholds) Check(score MutationScore) []Breach {
	breaches := appendBelow(nil, t.Key+".total", "", percent(score.Score()), t.Total)

	// Equivalent: max survivors are validated to be 0 or more, so > 0 is != 0.
	if t.MaxSurvivors > 0 && score.Survived > t.MaxSurvivors { //gooze:ignore comparison
		breaches = append(breaches, Breach{Key: maxSurvivorsKey, Score: float64(score.Survived), Min: float64(t.MaxSurvivors)})
	}

	files := slices.SortedFunc(slices.Values(score.Files), func(a, b FileSummary) int { return strings.Compare(a.Path, b.Path) })
	// Equivalent: a file's scored count is never negative, so == 0 is <= 0.
	files = slices.DeleteFunc(files, func(f FileSummary) bool { return f.Scored() == 0 }) //gooze:ignore comparison

	if len(files) > 0 {
		sum := 0.0
		for _, f := range files {
			sum += f.Score()
		}

		breaches = appendBelow(breaches, t.Key+".average", "", percent(sum/float64(len(files))), t.Average)
	}

	for _, f := range files {
		breaches = appendBelow(breaches, t.Key+".file", f.Path, percent(f.Score()), t.File)
	}

	return breaches
}

func appendBelow(breaches []Breach, key, file string, score, minimum float64) []Breach {
	if score >= minimum {
		return breaches
	}

	return append(breaches, Breach{Key: key, File: file, Score: score, Min: minimum})
}

// percent turns a 0..1 score into a percent rounded to two decimals, as
// gooze prints it.
func percent(score float64) float64 {
	return math.Round(score*10000) / 100
}
