package domain

import (
	"context"
	"errors"
	"fmt"
	"os"

	m "gooze.dev/pkg/gooze/internal/model"
	pkg "gooze.dev/pkg/gooze/pkg"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// cachedSelection returns the stored results a narrowed, unsharded run with
// the cache on selects in the files it does not re-test, scoped like the
// run's own, so the run scores and saves them as cached. It returns nil for
// any other run.
func (w *workflow) cachedSelection(ctx context.Context, args TestArgs, sel Selection, sources sourceSet) (pkg.FileSpill[m.Report], error) {
	if !sel.Narrowed() || args.Sharded() {
		return nil, nil //nolint:nilnil // nil: the run reuses no stored results.
	}

	selected, err := w.selectedCached(ctx, sel, sources)
	if err != nil {
		return nil, fmt.Errorf("select cached mutations: %w", err)
	}

	// Equivalent: a length is never below 0, so <= 0 is == 0.
	if len(selected) == 0 { //gooze:ignore comparison
		return nil, nil //nolint:nilnil // nil: no file is cached.
	}

	stored, err := w.reports.LoadSpillReports(ctx, args.Reports)
	if errors.Is(err, os.ErrNotExist) {
		return pkg.NewFileSpill[m.Report]()
	}

	if err != nil {
		return nil, fmt.Errorf("load reports: %w", err)
	}

	defer closeSpill("stored reports", stored)

	return selectReports(stored, selected, sel)
}

// selectedCached returns the source key and mutationKey of every mutation sel selects in the
// sources in scope that the run does not re-test.
func (w *workflow) selectedCached(ctx context.Context, sel Selection, sources sourceSet) (map[string]bool, error) {
	changed, err := sourceKeys(sources.changed)
	if err != nil {
		return nil, err
	}

	selected := map[string]bool{}

	err = sources.scanned.Range(func(_ uint64, source rpt.Source) error {
		if _, ok := changed[source.Key()]; ok || !sel.InScope(source) {
			return nil
		}

		return w.streamMutations(ctx, source, sel.Options, func(mutation m.Mutation) error {
			if sel.Includes(mutation) {
				selected[source.Key()+"/"+mutationKey(mutation.Type.Name, mutation.Type.Version, mutation.ID)] = true
			}

			return nil
		}, sel.Types()...)
	})

	return selected, err
}

// selectReports returns the results of stored that are in selected, with the
// scope sel gives their source.
func selectReports(stored pkg.FileSpill[m.Report], selected map[string]bool, sel Selection) (pkg.FileSpill[m.Report], error) {
	reports, err := pkg.NewFileSpill[m.Report]()
	if err != nil {
		return nil, fmt.Errorf("create cached reports spill: %w", err)
	}

	err = stored.Range(func(_ uint64, report m.Report) error {
		result := m.Result{}

		for typ, results := range report.Result {
			for _, res := range results {
				if selected[report.Source.Key()+"/"+mutationKey(typ.Name, typ.Version, res.MutationID)] {
					result[typ] = append(result[typ], res)
				}
			}
		}

		if len(result) == 0 {
			return nil
		}

		report.Result = result
		report.Scope = sel.ReportScope(report.Source)

		return reports.Append(report)
	})
	if err != nil {
		closeSpill("cached reports", reports)
		return nil, err
	}

	return reports, nil
}
