package domain

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	m "gooze.dev/pkg/gooze/internal/model"
	pkg "gooze.dev/pkg/gooze/pkg"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// runFile is what a narrowed run tested of one source file.
type runFile struct {
	source    rpt.Source
	mutations map[string]bool // mutagen name + "/" + mutation ID
}

// baseFile is what the main reports hold of a source file the run tested.
type baseFile struct {
	unchanged bool // every main report has the run's origin and test hashes
	partial   bool // some main report covers only part of the file
}

// mergeRunReports folds a narrowed run's reports (base/run and its shard
// dirs) into base, then removes them. For a file unchanged since the main
// reports, the run's results replace the matching mutations and the rest are
// kept, so a complete file stays complete. For a changed or new file, the
// run's results replace the file's and keep their partial scope, so the next
// full run re-tests it. Without a run directory it does nothing.
func (w *workflow) mergeRunReports(ctx context.Context, base m.Path) error {
	runDir := filepath.Join(string(base), RunDirName)

	shardDirs, err := findShardDirs(runDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("find run shard directories: %w", err)
	}

	runReports, err := w.loadShardReports(ctx, append([]string{runDir}, shardDirs...))
	if err != nil {
		return err
	}

	defer closeSpill("run reports", runReports)

	if err := w.overlayRunReports(ctx, base, runReports); err != nil {
		return err
	}

	if err := os.RemoveAll(runDir); err != nil {
		return fmt.Errorf("remove %s: %w", runDir, err)
	}

	return nil
}

func (w *workflow) overlayRunReports(ctx context.Context, base m.Path, runReports pkg.FileSpill[m.Report]) error {
	files, err := indexRunReports(runReports)
	if err != nil {
		return err
	}

	stored, err := w.storedRunFiles(ctx, base, files)
	if err != nil {
		return err
	}

	kept, stale, err := w.keptBaseReports(ctx, base, files, stored)
	if err != nil {
		return err
	}

	defer closeSpill("kept reports", kept)
	defer closeSpill("stale sources", stale)

	if err := w.reports.CleanReports(ctx, base, stale); err != nil {
		return fmt.Errorf("clean superseded reports: %w", err)
	}

	merged, err := withMergedScope(runReports, stored)
	if err != nil {
		return err
	}

	defer closeSpill("merged run reports", merged)

	for _, reports := range []pkg.FileSpill[m.Report]{kept, merged} {
		if err := w.reports.SaveReports(ctx, base, reports); err != nil {
			return fmt.Errorf("save merged reports: %w", err)
		}
	}

	return w.regenerateIndex(ctx, base)
}

// indexRunReports groups the run's mutations by source file.
func indexRunReports(reports pkg.FileSpill[m.Report]) (map[string]*runFile, error) {
	files := map[string]*runFile{}

	err := reports.Range(func(_ uint64, report m.Report) error {
		key := report.Source.Key()

		file, ok := files[key]
		if !ok {
			file = &runFile{source: report.Source, mutations: map[string]bool{}}
			files[key] = file
		}

		for _, id := range reportMutationKeys(report) {
			file.mutations[id] = true
		}

		return nil
	})

	return files, err
}

func reportMutationKeys(report m.Report) []string {
	var keys []string

	for mutagen, results := range report.Result {
		for _, result := range results {
			keys = append(keys, mutagen.Name+"/"+result.MutationID)
		}
	}

	return keys
}

// storedRunFiles describes the main reports of every file the run tested.
func (w *workflow) storedRunFiles(ctx context.Context, base m.Path, files map[string]*runFile) (map[string]baseFile, error) {
	stored := map[string]baseFile{}

	err := w.rangeBaseline(ctx, base, func(report m.Report) error {
		key := report.Source.Key()

		file, ok := files[key]
		if !ok {
			return nil
		}

		state, seen := stored[key]
		state.unchanged = (!seen || state.unchanged) && sameHashes(report.Source, file.source)
		state.partial = state.partial || report.Scope != nil
		stored[key] = state

		return nil
	})

	return stored, err
}

// keptBaseReports returns the main reports to keep for the run's files (those
// of unchanged files that the run did not re-test) and the sources whose main
// reports are to be replaced.
func (w *workflow) keptBaseReports(ctx context.Context, base m.Path, files map[string]*runFile, stored map[string]baseFile) (pkg.FileSpill[m.Report], pkg.FileSpill[rpt.Source], error) {
	kept, err := pkg.NewFileSpill[m.Report]()
	if err != nil {
		return nil, nil, fmt.Errorf("create kept reports spill: %w", err)
	}

	stale, err := pkg.NewFileSpill[rpt.Source]()
	if err != nil {
		closeSpill("kept reports", kept)
		return nil, nil, fmt.Errorf("create stale sources spill: %w", err)
	}

	addStale := uniqueSourceAppender(stale)

	err = w.rangeBaseline(ctx, base, func(report m.Report) error {
		file, ok := files[report.Source.Key()]
		if !ok {
			return nil
		}

		if err := addStale(report.Source); err != nil {
			return err
		}

		if !stored[report.Source.Key()].unchanged || overlaps(report, file) {
			return nil
		}

		return kept.Append(report)
	})
	if err != nil {
		closeSpill("kept reports", kept)
		closeSpill("stale sources", stale)

		return nil, nil, err
	}

	return kept, stale, nil
}

func overlaps(report m.Report, file *runFile) bool {
	for _, key := range reportMutationKeys(report) {
		if file.mutations[key] {
			return true
		}
	}

	return false
}

// withMergedScope returns the run's reports with their scope cleared where
// the merge leaves the file complete: unchanged and fully tested before.
func withMergedScope(runReports pkg.FileSpill[m.Report], stored map[string]baseFile) (pkg.FileSpill[m.Report], error) {
	merged, err := pkg.NewFileSpill[m.Report]()
	if err != nil {
		return nil, fmt.Errorf("create merged reports spill: %w", err)
	}

	err = runReports.Range(func(_ uint64, report m.Report) error {
		if state, ok := stored[report.Source.Key()]; ok && state.unchanged && !state.partial {
			report.Scope = nil
		}

		return merged.Append(report)
	})
	if err != nil {
		closeSpill("merged run reports", merged)
		return nil, err
	}

	return merged, nil
}

// sameHashes reports whether a and b describe the same origin and test file
// content.
func sameHashes(a, b rpt.Source) bool {
	return fileHash(a.Origin) == fileHash(b.Origin) && fileHash(a.Test) == fileHash(b.Test)
}

func fileHash(f *rpt.File) string {
	if f == nil {
		return ""
	}

	return f.Hash
}
