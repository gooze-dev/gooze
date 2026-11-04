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

// cachedSelection returns the stored results a run reuses and saves with its
// own, so it scores them as cached: a narrowed run's are those it selects in
// the files it does not re-test, scoped like its own results. A shard keeps
// only the sources it owns (see ownsSource), and a full shard all their
// stored results, so `report merge` of the shards gives the unsharded score.
// It returns nil for a full, unsharded run, which scores the stored reports
// in place.
func (w *workflow) cachedSelection(ctx context.Context, args TestArgs, sel Selection, sources sourceSet) (pkg.FileSpill[m.Report], error) {
	if !sel.Narrowed() {
		if !args.Sharded() || !sources.cached {
			return nil, nil //nolint:nilnil // nil: the run reuses no stored results.
		}

		return w.shardStored(ctx, args, sources)
	}

	selected, err := w.selectedCached(ctx, sel, sources, func(source rpt.Source) bool { return ownsSource(args, source) })
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

// shardStored returns, unchanged, the stored reports of the unchanged sources
// in scope that the shard of args owns.
func (w *workflow) shardStored(ctx context.Context, args TestArgs, sources sourceSet) (pkg.FileSpill[m.Report], error) {
	stored, err := w.reports.LoadSpillReports(ctx, args.Reports)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil //nolint:nilnil // nil: there is no baseline.
	}

	if err != nil {
		return nil, fmt.Errorf("load reports: %w", err)
	}

	defer closeSpill("stored reports", stored)

	return unchangedInScope(stored, sources, func(source rpt.Source) bool { return ownsSource(args, source) })
}

// ownsSource reports whether the shard of args keeps the stored results of
// source. Each source belongs to exactly one shard, picked like a mutant's
// (see inShard) but by source key, so a cached source's reports are copied
// whole into one shard dir. An unsharded run owns every source.
func ownsSource(args TestArgs, source rpt.Source) bool {
	return inShard(source.Key(), args.ShardIndex, args.TotalShardCount)
}

// selectedCached returns the source key and mutationKey of every mutation sel selects in the
// sources in scope that the run does not re-test and owns.
func (w *workflow) selectedCached(ctx context.Context, sel Selection, sources sourceSet, owns func(rpt.Source) bool) (map[string]bool, error) {
	changed, err := sourceKeys(sources.changed)
	if err != nil {
		return nil, err
	}

	selected := map[string]bool{}

	err = sources.scanned.Range(func(_ uint64, source rpt.Source) error {
		if _, ok := changed[source.Key()]; ok || !sel.InScope(source) || !owns(source) {
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
