package domain

import (
	"context"
	"fmt"
	"sync"

	m "gooze.dev/pkg/gooze/internal/model"
	pkg "gooze.dev/pkg/gooze/pkg"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// FailFast is when a run stops before testing every mutant.
type FailFast string

const (
	// FailFastOff tests every mutant.
	FailFastOff FailFast = "off"
	// FailFastGate stops once a quality gate can no longer be met.
	FailFastGate FailFast = "gate"
	// FailFastFile skips the rest of a file that can't meet threshold.file.
	FailFastFile FailFast = "file"
	// FailFastFirst stops at the first surviving mutant.
	FailFastFirst FailFast = "first"
)

// FailFastError is the verdict of a run FailFastFirst stopped.
type FailFastError struct {
	Position string
}

func (e *FailFastError) Error() string {
	return fmt.Sprintf("stopped at the first surviving mutant, %s (fail-fast %s)", e.Position, FailFastFirst)
}

// fileTally counts a file's results so far and its untested mutants.
type fileTally struct {
	rpt.Summary

	untested int
}

// best counts every untested mutant as killed.
func (t fileTally) best() rpt.Summary {
	best := t.Summary
	best.Total += t.untested
	best.Killed += t.untested

	return best
}

// failFastWatch applies a fail-fast policy to a run's results as they come
// in. A nil watch never acts.
type failFastWatch struct {
	policy FailFast
	gates  Thresholds
	stop   context.CancelCauseFunc

	mu      sync.Mutex
	files   map[string]*fileTally // by source key
	skipped map[string]bool
	acted   bool
}

// newFailFastWatch returns nil when the policy can't act. On a shard only the
// survivor count is meaningful, since merging can only raise it.
func newFailFastWatch(policy FailFast, gates Thresholds, sharded bool, stop context.CancelCauseFunc) *failFastWatch {
	switch policy {
	case FailFastGate:
		if sharded {
			gates = Thresholds{Key: gates.Key, MaxSurvivors: gates.MaxSurvivors}
		}
	case FailFastFile:
		gates = Thresholds{Key: gates.Key, File: gates.File}
		if sharded {
			gates = Thresholds{}
		}
	case FailFastFirst:
		gates = Thresholds{}
	case FailFastOff:
		return nil
	default:
		return nil
	}

	if policy != FailFastFirst && !gates.Set() {
		return nil
	}

	return &failFastWatch{policy: policy, gates: gates, stop: stop, files: map[string]*fileTally{}, skipped: map[string]bool{}}
}

// plan counts the untested mutants per file and adds the scored stored results.
func (f *failFastWatch) plan(mutations pkg.FileSpill[m.Mutation], scored pkg.FileSpill[m.Report]) error {
	if f == nil {
		return nil
	}

	err := mutations.Range(func(_ uint64, mutation m.Mutation) error {
		f.file(mutation.Source.Key()).untested++
		return nil
	})
	if err != nil || scored == nil {
		return err
	}

	return scored.Range(func(_ uint64, report m.Report) error {
		f.file(report.Source.Key()).Merge(summarizeReport(report))
		return nil
	})
}

func (f *failFastWatch) file(key string) *fileTally {
	tally, ok := f.files[key]
	if !ok {
		tally = &fileTally{}
		f.files[key] = tally
	}

	return tally
}

// record counts a mutation's result and acts on it.
func (f *failFastWatch) record(mutation m.Mutation, result m.Result) {
	if f == nil {
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	status := getMutationStatus(result, mutation)

	var counted rpt.Summary

	counted.Add(status)

	key := mutation.Source.Key()
	tally := f.file(key)
	tally.Merge(counted)
	tally.untested--

	switch f.policy {
	case FailFastFirst:
		if status == rpt.Survived {
			f.halt(&FailFastError{Position: fmt.Sprintf("%s:%d", key, mutation.Line)})
		}
	case FailFastGate:
		if breaches := f.gates.Check(f.bestScore()); len(breaches) > 0 {
			f.halt(WithExitCode(m.ExitBelowThreshold, &ThresholdError{Breaches: breaches}))
		}
	case FailFastFile:
		best := FileSummary{Path: key, Summary: tally.best()}
		// Equivalent: untested counts are never negative, so > 0 is != 0.
		if tally.untested > 0 && len(f.gates.Check(MutationScore{Files: []FileSummary{best}})) > 0 { //gooze:ignore comparison
			f.skipped[best.Path] = true
			f.acted = true
		}
	case FailFastOff:
	}
}

// halt stops the run once.
func (f *failFastWatch) halt(verdict error) {
	if f.acted {
		return
	}

	f.acted = true
	f.stop(verdict)
}

// bestScore counts every untested mutant as killed.
func (f *failFastWatch) bestScore() MutationScore {
	var score MutationScore

	for key, tally := range f.files {
		best := tally.best()
		score.Merge(best)
		score.Files = append(score.Files, FileSummary{Path: key, Summary: best})
	}

	return score
}

// skips reports whether the file policy gave up on key.
func (f *failFastWatch) skips(key string) bool {
	if f == nil {
		return false
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	return f.skipped[key]
}

// actedOn returns the policy if it left mutants untested, or "".
func (f *failFastWatch) actedOn() FailFast {
	if f == nil {
		return ""
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if !f.acted {
		return ""
	}

	return f.policy
}

// failFastWatch returns the run's watch, planned with the mutants to test and
// the cached and resumed results it scores.
func (w *workflow) failFastWatch(
	args TestArgs,
	mutations pkg.FileSpill[m.Mutation],
	stored pkg.FileSpill[m.Report],
	resumed resumedFiles,
	sources sourceSet,
	stop context.CancelCauseFunc,
) (*failFastWatch, error) {
	// Equivalent: the CLI passes a total of at least 1, so > 1 is != 1.
	watch := newFailFastWatch(args.FailFast, args.Thresholds, args.TotalShardCount > 1, stop) //gooze:ignore comparison
	if watch == nil {
		return nil, nil //nolint:nilnil // nil: the policy can't act.
	}

	if stored == nil {
		return watch, watch.plan(mutations, nil)
	}

	scored, err := pkg.NewFileSpill[m.Report]()
	if err != nil {
		return nil, fmt.Errorf("create reports spill: %w", err)
	}

	defer closeSpill("fail-fast reports", scored)

	for _, reports := range resumed.reports {
		for _, report := range reports {
			if err := scored.Append(report); err != nil {
				return nil, err
			}
		}
	}

	cached, err := reportsInScope(stored, scored, sources)
	if err != nil {
		return nil, err
	}

	defer closeSpill("cached reports", cached)

	return watch, watch.plan(mutations, cached)
}
