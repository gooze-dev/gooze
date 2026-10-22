// Package adapter contains UI and infrastructure adapters for the Gooze CLI.
package adapter

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"gopkg.in/yaml.v3"

	m "gooze.dev/pkg/gooze/internal/model"
	"gooze.dev/pkg/gooze/pkg"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// LocalReportStore stores reports as YAML files on the local filesystem.
type LocalReportStore struct{}

// NewReportStore constructs a LocalReportStore instance ready to
// be wired into the workflow.
func NewReportStore() *LocalReportStore {
	return &LocalReportStore{}
}

// SaveReports writes one YAML file per report from the filespill into the provided directory.
func (rs *LocalReportStore) SaveReports(ctx context.Context, path m.Path, reports pkg.FileSpill[m.Report]) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	dirPath := string(path)
	if dirPath == "" {
		return fmt.Errorf("reports directory path is required")
	}

	if err := os.MkdirAll(dirPath, 0o750); err != nil {
		return fmt.Errorf("create reports directory: %w", err)
	}

	return reports.Range(func(_ uint64, report m.Report) error {
		reportHash := rs.computeReportHash(report.Result)
		if reportHash == "" {
			return nil
		}

		data, err := rs.marshalReport(report)
		if err != nil {
			return fmt.Errorf("marshal report to YAML: %w", err)
		}

		name := reportHash + ".yaml"

		fullPath := filepath.Join(dirPath, name)
		if err := os.WriteFile(fullPath, data, 0o600); err != nil {
			return fmt.Errorf("write report file %s: %w", fullPath, err)
		}

		return nil
	})
}

// RegenerateIndex rebuilds and writes `_index.yaml` from the report files in `path`.
func (rs *LocalReportStore) RegenerateIndex(ctx context.Context, path m.Path) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	dirPath := string(path)
	if dirPath == "" {
		return fmt.Errorf("reports directory path is required")
	}

	exists, err := rs.reportsDirExists(dirPath)
	if err != nil {
		return err
	}

	if !exists {
		return nil
	}

	builder := newIndexBuilder(rs)
	if err := rs.rangeReportsInDir(dirPath, func(report m.Report) error {
		builder.add(report)
		return nil
	}); err != nil {
		return err
	}

	return rs.writeIndex(dirPath, builder)
}

func (rs *LocalReportStore) reportsDirExists(dirPath string) (bool, error) {
	if err := rs.validateReportsDir(dirPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}

		return false, err
	}

	return true, nil
}

func (rs *LocalReportStore) rangeReportsInDir(dirPath string, fn func(m.Report) error) error {
	return rpt.WalkDir(dirPath, func(_ string, report rpt.Report) error {
		return fn(decodeReport(report))
	})
}

func (rs *LocalReportStore) writeIndex(dirPath string, builder *indexBuilder) error {
	indexPath := filepath.Join(dirPath, rpt.IndexFileName)
	// Equivalent: count only goes up from 0, so <= 0 is == 0.
	if builder.count == 0 { //gooze:ignore comparison
		if err := os.Remove(indexPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove index file %s: %w", indexPath, err)
		}

		return nil
	}

	indexData, err := yaml.Marshal(builder.build())
	if err != nil {
		return fmt.Errorf("marshal index YAML: %w", err)
	}

	if err := os.WriteFile(indexPath, indexData, 0o600); err != nil {
		return fmt.Errorf("write index file %s: %w", indexPath, err)
	}

	return nil
}

// SaveSelection writes selection to rpt.SelectionFileName in path, which
// SaveReports has created.
func (rs *LocalReportStore) SaveSelection(ctx context.Context, path m.Path, selection rpt.Selection) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	data, err := json.Marshal(selection)
	if err != nil {
		return fmt.Errorf("marshal selection: %w", err)
	}

	file := filepath.Join(string(path), rpt.SelectionFileName)
	if err := os.WriteFile(file, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", file, err)
	}

	return nil
}

// LoadSelection reads rpt.SelectionFileName from path; a missing file is an
// empty selection.
func (rs *LocalReportStore) LoadSelection(ctx context.Context, path m.Path) (rpt.Selection, error) {
	if err := ctx.Err(); err != nil {
		return rpt.Selection{}, err
	}

	file := filepath.Join(string(path), rpt.SelectionFileName)

	data, err := os.ReadFile(file) //nolint:gosec // the path is the configured reports directory.
	if errors.Is(err, os.ErrNotExist) {
		return rpt.Selection{}, nil
	}

	if err != nil {
		return rpt.Selection{}, fmt.Errorf("read %s: %w", file, err)
	}

	var selection rpt.Selection
	if err := json.Unmarshal(data, &selection); err != nil {
		return rpt.Selection{}, fmt.Errorf("parse %s: %w", file, err)
	}

	return selection, nil
}

// LoadSpillReports loads previously saved reports from disk into a FileSpill.
func (rs *LocalReportStore) LoadSpillReports(ctx context.Context, path m.Path) (pkg.FileSpill[m.Report], error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	dirPath := string(path)
	if dirPath == "" {
		return nil, fmt.Errorf("reports directory path is required")
	}

	if err := rs.validateReportsDir(dirPath); err != nil {
		return nil, err
	}

	spill, err := pkg.NewFileSpill[m.Report]()
	if err != nil {
		return nil, fmt.Errorf("create filespill: %w", err)
	}

	if err := rs.rangeReportsInDir(dirPath, spill.Append); err != nil {
		return nil, errors.Join(fmt.Errorf("load reports into spill: %w", err), spill.Close())
	}

	return spill, nil
}

type storedSourceState struct {
	source  rpt.Source
	mutator map[string]int
	// partial is set when any stored report covers only some lines of the
	// source (a diff-mode run), so its other lines were never tested.
	partial bool
}

// CleanReports deletes stored report files that belong to the provided sources.
// It is safe to call when the reports directory does not exist.
func (rs *LocalReportStore) CleanReports(ctx context.Context, path m.Path, sources pkg.FileSpill[rpt.Source]) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	dirPath := string(path)
	if dirPath == "" {
		return fmt.Errorf("reports directory path is required")
	}

	exists, err := rs.reportsDirExists(dirPath)
	if err != nil {
		return err
	}

	if !exists {
		return nil
	}

	toCleanPaths, toCleanHashes, err := rs.buildCleanMatchers(sources)
	if err != nil {
		return err
	}

	return rs.cleanReportsIn(dirPath, toCleanPaths, toCleanHashes)
}

func (rs *LocalReportStore) cleanReportsIn(dirPath string, toCleanPaths, toCleanHashes map[string]bool) error {
	// Equivalent: a length is never below 0, and with both matchers empty the walk below removes nothing either.
	if len(toCleanPaths) == 0 && len(toCleanHashes) == 0 { //gooze:ignore comparison
		return nil
	}

	return rpt.WalkDir(dirPath, func(filePath string, report rpt.Report) error {
		if !rs.shouldCleanReport(report.Source, toCleanPaths, toCleanHashes) {
			return nil
		}

		if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove report file %s: %w", filePath, err)
		}

		return nil
	})
}

func (rs *LocalReportStore) buildCleanMatchers(sources pkg.FileSpill[rpt.Source]) (map[string]bool, map[string]bool, error) {
	paths := make(map[string]bool)
	hashes := make(map[string]bool)

	if sources == nil {
		return paths, hashes, nil
	}

	err := sources.Range(func(_ uint64, src rpt.Source) error {
		if src.Origin == nil {
			return nil
		}

		// Equivalent: >= "" is always true, and an empty key never matches, since shouldCleanReport skips it.
		if key := src.Key(); key != "" { //gooze:ignore comparison
			paths[key] = true
		}

		// Equivalent: >= "" is always true, and an empty hash is never looked up, since shouldCleanReport skips it.
		if src.Origin.Hash != "" { //gooze:ignore comparison
			hashes[src.Origin.Hash] = true
		}

		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("read sources to clean: %w", err)
	}

	return paths, hashes, nil
}

func (rs *LocalReportStore) shouldCleanReport(source rpt.Source, toCleanPaths map[string]bool, toCleanHashes map[string]bool) bool {
	if source.Origin == nil {
		return false
	}

	// Equivalent: >= "" is always true, and "" is never in toCleanPaths (buildCleanMatchers skips it).
	if key := source.Key(); key != "" && toCleanPaths[key] { //gooze:ignore comparison
		return true
	}

	// Equivalent: >= "" is always true, and "" is never in toCleanHashes (buildCleanMatchers skips it).
	if source.Origin.Hash != "" { //gooze:ignore comparison
		if toCleanHashes[source.Origin.Hash] {
			return true
		}
	}

	return false
}

// CheckUpdates splits sources into those that must be re-tested and the stored
// sources whose files no longer exist. A source is re-tested when it has no
// stored reports, its source or test hash changed, the mutators that produced
// its reports changed, or untested (when set) returns true for it. Both
// returned spills must be closed by the caller.
func (rs *LocalReportStore) CheckUpdates(
	ctx context.Context,
	path m.Path,
	sources pkg.FileSpill[rpt.Source],
	untested func(source rpt.Source, tested []string) bool,
) (pkg.FileSpill[rpt.Source], pkg.FileSpill[rpt.Source], error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	dirPath := string(path)
	if dirPath == "" {
		return nil, nil, fmt.Errorf("reports directory path is required")
	}

	exists, err := rs.reportsDirExists(dirPath)
	if err != nil {
		return nil, nil, err
	}

	stored := make(map[string]storedSourceState)

	if exists {
		if err := rs.rangeReportsInDir(dirPath, func(report m.Report) error {
			rs.addStoredReport(stored, report)
			return nil
		}); err != nil {
			return nil, nil, err
		}
	}

	return rs.splitChangedAndDeleted(stored, sources, untested)
}

func (rs *LocalReportStore) splitChangedAndDeleted(
	stored map[string]storedSourceState,
	sources pkg.FileSpill[rpt.Source],
	untested func(rpt.Source, []string) bool,
) (pkg.FileSpill[rpt.Source], pkg.FileSpill[rpt.Source], error) {
	changed, err := pkg.NewFileSpill[rpt.Source]()
	if err != nil {
		return nil, nil, fmt.Errorf("create changed sources spill: %w", err)
	}

	deleted, err := pkg.NewFileSpill[rpt.Source]()
	if err != nil {
		return nil, nil, errors.Join(fmt.Errorf("create deleted sources spill: %w", err), changed.Close())
	}

	visited := make(map[string]bool)

	err = sources.Range(func(_ uint64, src rpt.Source) error {
		key := src.Key()
		visited[key] = true

		if rs.isSourceChanged(stored, key, src) || isUntested(stored[key], src, untested) {
			return changed.Append(src)
		}

		return nil
	})
	if err == nil {
		err = appendDeleted(stored, visited, deleted)
	}

	if err != nil {
		return nil, nil, errors.Join(fmt.Errorf("split changed sources: %w", err), changed.Close(), deleted.Close())
	}

	return changed, deleted, nil
}

func appendDeleted(stored map[string]storedSourceState, visited map[string]bool, deleted pkg.FileSpill[rpt.Source]) error {
	keys := make([]string, 0, len(stored))
	for key, st := range stored {
		if !visited[key] && !sourceFileExists(st.source) {
			keys = append(keys, key)
		}
	}

	sort.Strings(keys)

	for _, key := range keys {
		if err := deleted.Append(stored[key].source); err != nil {
			return err
		}
	}

	return nil
}

// sourceFileExists reports whether the source's file is still on disk, by its
// project-relative path from the working directory or by its absolute path.
func sourceFileExists(source rpt.Source) bool {
	if source.Origin == nil {
		return false
	}

	for _, candidate := range []string{source.Origin.ShortPath, source.Origin.FullPath} {
		if _, err := os.Stat(candidate); err == nil {
			return true
		}
	}

	return false
}

func (rs *LocalReportStore) isSourceChanged(stored map[string]storedSourceState, pathStr string, current rpt.Source) bool {
	st, ok := stored[pathStr]
	if !ok {
		return true
	}

	if st.partial || rs.sourceHashChanged(st.source, current) {
		return true
	}

	return rs.mutatorsChanged(st.mutator)
}

// isUntested asks untested about a source whose stored reports are current.
func isUntested(st storedSourceState, current rpt.Source, untested func(rpt.Source, []string) bool) bool {
	if untested == nil {
		return false
	}

	tested := make([]string, 0, len(st.mutator))
	for name := range st.mutator {
		tested = append(tested, name)
	}

	sort.Strings(tested)

	return untested(current, tested)
}

func (rs *LocalReportStore) addStoredReport(state map[string]storedSourceState, report m.Report) {
	key := report.Source.Key()
	if key == "" {
		return
	}

	st, ok := state[key]
	if !ok {
		st = storedSourceState{source: report.Source, mutator: map[string]int{}}
	}

	// Keep the most recently seen Source metadata (hashes), but they should be consistent.
	st.source = report.Source
	st.partial = st.partial || report.Scope != nil

	for mt := range report.Result {
		if existing, ok := st.mutator[mt.Name]; ok && existing != mt.Version {
			// Version mismatch across reports - mark as needing update
			// Use -1 as a sentinel to indicate inconsistency
			st.mutator[mt.Name] = -1
		} else if !ok {
			st.mutator[mt.Name] = mt.Version
		}
	}

	state[key] = st
}

func (rs *LocalReportStore) sourceHashChanged(stored rpt.Source, current rpt.Source) bool {
	storedOriginHash := ""
	currentOriginHash := ""

	if stored.Origin != nil {
		storedOriginHash = stored.Origin.Hash
	}

	if current.Origin != nil {
		currentOriginHash = current.Origin.Hash
	}

	if storedOriginHash != currentOriginHash {
		return true
	}

	storedTestHash := ""
	currentTestHash := ""

	if stored.Test != nil {
		storedTestHash = stored.Test.Hash
	}

	if current.Test != nil {
		currentTestHash = current.Test.Hash
	}

	if storedTestHash != currentTestHash {
		return true
	}

	return false
}

func (rs *LocalReportStore) mutatorsChanged(stored map[string]int) bool {
	current := mgen.Versions()

	// Check if any mutator versions changed for mutators that were stored
	for name, storedVersion := range stored {
		// -1 indicates version mismatch across reports - needs re-run
		// Equivalent: a shortcut: -1, like any stored version that isn't the current one, returns true below anyway.
		if storedVersion == -1 { //gooze:ignore comparison, numbers
			return true
		}

		currentVersion, ok := current[name]
		if !ok {
			return true
		}

		if storedVersion != currentVersion {
			return true
		}
	}

	return false
}

func (rs *LocalReportStore) marshalReport(report m.Report) ([]byte, error) {
	return yaml.Marshal(rpt.Report{
		Source: report.Source,
		Result: encodeResult(report.Result),
		Diff:   report.Diff,
		Line:   report.Line,
		Scope:  report.Scope,
	})
}

func decodeReport(report rpt.Report) m.Report {
	return m.Report{
		Source: report.Source,
		Result: decodeResult(report.Result),
		Diff:   report.Diff,
		Line:   report.Line,
		Scope:  report.Scope,
	}
}

func encodeResult(result m.Result) []rpt.Result {
	// Equivalent: a shortcut: an empty result gives an empty, non-nil slice below too.
	if len(result) == 0 { //gooze:ignore comparison
		return []rpt.Result{}
	}

	keys := make([]mgen.Type, 0, len(result))
	for mutationType := range result {
		keys = append(keys, mutationType)
	}

	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Name != keys[j].Name {
			// Equivalent: only reached when the names differ, so <= is <.
			return keys[i].Name < keys[j].Name //gooze:ignore comparison
		}

		// Equivalent: two map keys never share a name and a version, so <= is <.
		return keys[i].Version < keys[j].Version //gooze:ignore comparison
	})

	entries := make([]rpt.Result, 0, len(keys))
	for _, mutationType := range keys {
		results := result[mutationType]
		entry := rpt.Result{
			Name:      mutationType.Name,
			Version:   mutationType.Version,
			Mutations: make([]rpt.MutationResult, 0, len(results)),
		}

		for _, res := range results {
			errString := ""
			if res.Err != nil {
				errString = res.Err.Error()
			}

			entry.Mutations = append(entry.Mutations, rpt.MutationResult{
				MutationID: res.MutationID,
				Status:     res.Status,
				Err:        errString,
			})
		}

		entries = append(entries, entry)
	}

	return entries
}

func decodeResult(entries []rpt.Result) m.Result {
	// Equivalent: a shortcut: no entries give an empty result below too.
	if len(entries) == 0 { //gooze:ignore comparison
		return m.Result{}
	}

	result := m.Result{}

	for _, entry := range entries {
		mutationType := mgen.Type{Name: entry.Name, Version: entry.Version}
		result[mutationType] = make([]struct {
			MutationID string
			Status     rpt.TestStatus
			Err        error
		}, 0, len(entry.Mutations))

		for _, mut := range entry.Mutations {
			result[mutationType] = append(result[mutationType], struct {
				MutationID string
				Status     rpt.TestStatus
				Err        error
			}{
				MutationID: mut.MutationID,
				Status:     mut.Status,
				Err:        nil,
			})
		}
	}

	return result
}

type indexState struct {
	mutationsBySource map[string]map[string]*rpt.IndexMutation
}

type indexBuilder struct {
	rs    *LocalReportStore
	index rpt.Index
	state indexState
	count int
}

func newIndexBuilder(rs *LocalReportStore) *indexBuilder {
	return &indexBuilder{
		rs:    rs,
		state: indexState{mutationsBySource: make(map[string]map[string]*rpt.IndexMutation)},
	}
}

func (b *indexBuilder) add(report m.Report) {
	b.count++

	sourceHex := b.rs.sourceHex(report.Source)
	reportFile := b.rs.computeReportHash(report.Result) + ".yaml"

	for mutationType, results := range report.Result {
		for _, result := range results {
			b.index.TotalMutations++
			b.rs.incrementStatusCount(&b.index, result.Status)
		}

		b.rs.trackMutationForIndex(&b.state, sourceHex, mutationType.Name, reportFile)
	}
}

func (b *indexBuilder) build() rpt.Index {
	for _, entries := range b.state.mutationsBySource {
		for _, ent := range entries {
			sort.Strings(ent.MutationReports)
			ent.MutationReports = slices.Compact(ent.MutationReports)
		}
	}

	b.index.Result = b.rs.buildIndexResults(b.state)

	return b.index
}

func (rs *LocalReportStore) trackMutationForIndex(state *indexState, sourceHex string, mutationName string, reportFile string) {
	if sourceHex == "" {
		return
	}

	entries := state.mutationsBySource[sourceHex]
	if entries == nil {
		entries = make(map[string]*rpt.IndexMutation)
		state.mutationsBySource[sourceHex] = entries
	}

	entry, exists := entries[mutationName]
	if !exists {
		entry = &rpt.IndexMutation{MutationName: mutationName, MutationReports: []string{}}
		entries[mutationName] = entry
	}

	entry.MutationReports = append(entry.MutationReports, reportFile)
}

func (rs *LocalReportStore) buildIndexResults(state indexState) []rpt.IndexResult {
	sourceHexes := make([]string, 0, len(state.mutationsBySource))
	for sourceHex := range state.mutationsBySource {
		sourceHexes = append(sourceHexes, sourceHex)
	}

	sort.Strings(sourceHexes)

	results := make([]rpt.IndexResult, 0, len(sourceHexes))
	for _, sourceHex := range sourceHexes {
		entries := state.mutationsBySource[sourceHex]

		names := make([]string, 0, len(entries))
		for name := range entries {
			names = append(names, name)
		}

		sort.Strings(names)

		out := rpt.IndexResult{SourceHex: sourceHex, Mutations: make([]rpt.IndexMutation, 0, len(names))}
		for _, name := range names {
			out.Mutations = append(out.Mutations, *entries[name])
		}

		results = append(results, out)
	}

	return results
}

func (rs *LocalReportStore) sourceHex(source rpt.Source) string {
	if source.Origin == nil {
		return ""
	}

	return source.Origin.Hash
}

func (rs *LocalReportStore) incrementStatusCount(index *rpt.Index, status rpt.TestStatus) {
	switch status {
	case rpt.Killed:
		index.KilledMutations++
	case rpt.Survived:
		index.SurvivedMutations++
	case rpt.Error, rpt.Timeout:
		index.FailedMutations++
	case rpt.Skipped:
		index.IgnoredMutations++
	case rpt.NotCovered:
		index.NotCoveredMutations++
	}
}

func (rs *LocalReportStore) validateReportsDir(dirPath string) error {
	info, err := os.Stat(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return os.ErrNotExist
		}

		return fmt.Errorf("stat reports directory: %w", err)
	}

	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", dirPath)
	}

	return nil
}

// computeReportHash generates a stable hash for a report based on its mutations.
func (rs *LocalReportStore) computeReportHash(result m.Result) string {
	if len(result) == 0 {
		return ""
	}

	parts := make([]string, 0)

	for mutationType, results := range result {
		for _, res := range results {
			parts = append(parts, res.MutationID+"|"+mutationType.Name)
		}
	}

	sort.Strings(parts)

	hash := sha256.Sum256([]byte(fmt.Sprintf("%v", parts)))

	return fmt.Sprintf("%x", hash)[:16]
}
