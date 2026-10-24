package domain

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"

	m "gooze.dev/pkg/gooze/internal/model"
	pkg "gooze.dev/pkg/gooze/pkg"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// testedRun is what the test pass of a run produced.
type testedRun struct {
	reports     pkg.FileSpill[m.Report]   // results of the mutants tested now
	mutations   pkg.FileSpill[m.Mutation] // the mutants the run was to test
	planned     int                       // how many there are
	interrupted bool                      // the run was stopped before testing them all
	failFast    FailFast                  // the fail-fast policy that left mutants untested, or ""
	stop        error                     // the verdict of a fail-fast stop, or nil
	stored      pkg.FileSpill[m.Report]   // the stored reports before the run, or nil
	resumed     resumedFiles
}

// partial reports whether the run left mutants untested.
func (r testedRun) partial() bool {
	return r.interrupted || r.failFast != ""
}

// storedReports loads the stored reports that a full, unsharded run with the
// cache on scores its cached files with and resumes interrupted files from.
// It returns nil for any other run.
func (w *workflow) storedReports(ctx context.Context, args TestArgs, sel Selection, sources sourceSet) (pkg.FileSpill[m.Report], error) {
	if !sources.cached || sel.Narrowed() || args.TotalShardCount > 1 {
		return nil, nil //nolint:nilnil // nil: the run keeps no stored reports.
	}

	stored, err := w.reports.LoadSpillReports(ctx, args.Reports)
	if errors.Is(err, os.ErrNotExist) {
		return pkg.NewFileSpill[m.Report]()
	}

	if err != nil {
		return nil, fmt.Errorf("load reports: %w", err)
	}

	return stored, nil
}

// reportsInScope returns the reports a full run scores once it has saved
// reports: the stored ones of the unchanged sources in scope, and reports.
func reportsInScope(stored, reports pkg.FileSpill[m.Report], sources sourceSet) (pkg.FileSpill[m.Report], error) {
	inScope, err := sourceKeys(sources.scanned)
	if err != nil {
		return nil, err
	}

	changed, err := sourceKeys(sources.changed)
	if err != nil {
		return nil, err
	}

	all, err := pkg.NewFileSpill[m.Report]()
	if err != nil {
		return nil, fmt.Errorf("create reports spill: %w", err)
	}

	err = stored.Range(func(_ uint64, report m.Report) error {
		key := report.Source.Key()
		if _, ok := inScope[key]; !ok {
			return nil
		}

		if _, ok := changed[key]; ok {
			return nil
		}

		return all.Append(report)
	})
	if err == nil {
		err = reports.Range(func(_ uint64, report m.Report) error { return all.Append(report) })
	}

	if err != nil {
		closeSpill("reports in scope", all)
		return nil, err
	}

	return all, nil
}

// resumedFiles holds the results an interrupted run saved for files that are
// unchanged since: the run tests only their other mutants and keeps these.
type resumedFiles struct {
	reports map[string][]m.Report // by source key
	tested  map[string]bool       // mutationKey of each result
}

// resumeFrom picks from stored (nil: none) the results of files a previous
// run left interrupted. A file's results are resumed only when its source and
// test are unchanged and every result is of a mutagen version this run uses;
// otherwise the file is tested in full.
func resumeFrom(stored pkg.FileSpill[m.Report], sel Selection, sources sourceSet) (resumedFiles, error) {
	resumed := resumedFiles{reports: map[string][]m.Report{}, tested: map[string]bool{}}
	if stored == nil {
		return resumed, nil
	}

	changed := map[string]rpt.Source{}
	if err := sources.changed.Range(func(_ uint64, source rpt.Source) error {
		changed[source.Key()] = source
		return nil
	}); err != nil {
		return resumedFiles{}, err
	}

	if err := interruptedReports(stored, changed, sel.Types(), resumed.reports); err != nil {
		return resumedFiles{}, fmt.Errorf("load interrupted results: %w", err)
	}

	for _, reports := range resumed.reports {
		for _, report := range reports {
			for typ, results := range report.Result {
				for _, result := range results {
					resumed.tested[mutationKey(typ.Name, typ.Version, result.MutationID)] = true
				}
			}
		}
	}

	return resumed, nil
}

// interruptedReports adds to into the stored reports of each changed source
// whose stored results can all be resumed.
func interruptedReports(stored pkg.FileSpill[m.Report], changed map[string]rpt.Source, types []mgen.Type, into map[string][]m.Report) error {
	stale := map[string]bool{}

	err := stored.Range(func(_ uint64, report m.Report) error {
		key := report.Source.Key()

		source, ok := changed[key]
		if !ok {
			return nil
		}

		if !interruptedOnly(report.Scope) || !sameHashes(report.Source, source) || !ofTypes(report, types) {
			stale[key] = true
		}

		into[key] = append(into[key], report)

		return nil
	})

	maps.DeleteFunc(into, func(key string, _ []m.Report) bool { return stale[key] })

	return err
}

// has reports whether mutation was tested by the interrupted run resumed.
func (r resumedFiles) has(mutation m.Mutation) bool {
	return r.tested[mutationKey(mutation.Type.Name, mutation.Type.Version, mutation.ID)]
}

func mutationKey(mutagen string, version int, id string) string {
	return fmt.Sprintf("%s/%d/%s", mutagen, version, id)
}

// interruptedOnly reports whether scope says only that the run was
// interrupted, with no other narrowing.
func interruptedOnly(scope *rpt.Scope) bool {
	if scope == nil {
		return false
	}

	return scope.Interrupted && scope.Base == "" && scope.Lines == nil && scope.Run == "" && scope.Mutagens == nil
}

func ofTypes(report m.Report, types []mgen.Type) bool {
	for typ := range report.Result {
		if !slices.Contains(types, typ) {
			return false
		}
	}

	return true
}

// unfinishedSources returns the keys of the sources some of whose mutants
// were left untested.
func unfinishedSources(run testedRun) (map[string]bool, error) {
	left := map[string]int{}

	if err := run.mutations.Range(func(_ uint64, mutation m.Mutation) error {
		left[mutation.Source.Key()]++
		return nil
	}); err != nil {
		return nil, err
	}

	if err := run.reports.Range(func(_ uint64, report m.Report) error {
		left[report.Source.Key()]--
		return nil
	}); err != nil {
		return nil, err
	}

	unfinished := map[string]bool{}

	for key, count := range left {
		// Equivalent mutants: a source has no more results than mutants.
		if count > 0 { //gooze:ignore comparison
			unfinished[key] = true
		}
	}

	return unfinished, nil
}

// completeReports returns the reports to save: the run's results and the
// resumed ones. The reports of a source some of whose mutants an interrupted
// or fail-fast run left untested are marked interrupted, so the next run
// tests the rest; the others are complete.
func completeReports(run testedRun) (pkg.FileSpill[m.Report], error) {
	unfinished := map[string]bool{}

	if run.partial() {
		var err error
		if unfinished, err = unfinishedSources(run); err != nil {
			return nil, err
		}
	}

	reports, err := pkg.NewFileSpill[m.Report]()
	if err != nil {
		return nil, fmt.Errorf("create reports spill: %w", err)
	}

	add := func(report m.Report, scope *rpt.Scope) error {
		if unfinished[report.Source.Key()] {
			scope = withInterrupted(scope)
		}

		report.Scope = scope

		return reports.Append(report)
	}

	err = run.reports.Range(func(_ uint64, report m.Report) error {
		return add(report, report.Scope)
	})

	for _, resumed := range run.resumed.reports {
		for _, report := range resumed {
			if err != nil {
				break
			}

			err = add(report, nil)
		}
	}

	if err != nil {
		closeSpill("reports", reports)
		return nil, err
	}

	return reports, nil
}

func withInterrupted(scope *rpt.Scope) *rpt.Scope {
	marked := rpt.Scope{}
	if scope != nil {
		marked = *scope
	}

	marked.Interrupted = true

	return &marked
}
