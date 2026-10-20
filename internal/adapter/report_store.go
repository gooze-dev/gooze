// Package adapter contains UI and infrastructure adapters for the Gooze CLI.
package adapter

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	m "gooze.dev/pkg/gooze/internal/model"
	"gooze.dev/pkg/gooze/pkg"
)

const indexFileName = "_index.yaml"

// LocalReportStore stores reports as YAML files on the local filesystem.
type LocalReportStore struct{}

// NewReportStore constructs a LocalReportStore instance ready to
// be wired into the workflow.
func NewReportStore() *LocalReportStore {
	return &LocalReportStore{}
}

type reportYAML struct {
	Source m.Source          `yaml:"source"`
	Result []resultEntryYAML `yaml:"result"`
	Diff   *[]byte           `yaml:"diff"`
}

type resultEntryYAML struct {
	Name      string               `yaml:"name"`
	Version   int                  `yaml:"version"`
	Mutations []mutationResultYAML `yaml:"mutations"`
}

type mutationResultYAML struct {
	MutationID string       `yaml:"mutationid"`
	Status     m.TestStatus `yaml:"status"`
	Err        string       `yaml:"err,omitempty"`
}

type mutationEntry struct {
	MutationName    string   `yaml:"mutation_name"`
	MutationReports []string `yaml:"mutation_reports"`
}

type resultEntry struct {
	SourceHex string          `yaml:"source_hex"`
	Mutations []mutationEntry `yaml:"mutations"`
}

type indexEntry struct {
	TotalMutations      int           `yaml:"total_mutations"`
	KilledMutations     int           `yaml:"killed_mutations"`
	SurvivedMutations   int           `yaml:"survived_mutations"`
	FailedMutations     int           `yaml:"failed_mutations"`
	IgnoredMutations    int           `yaml:"ignored_mutations"`
	NotCoveredMutations int           `yaml:"not_covered_mutations"`
	Result              []resultEntry `yaml:"result"`
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
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return fmt.Errorf("read reports directory: %w", err)
	}

	for _, entry := range entries {
		if !rs.shouldLoadReportEntry(entry) {
			continue
		}

		filePath := filepath.Join(dirPath, entry.Name())
		// #nosec G304 -- filePath is built from a trusted reports directory listing
		data, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("read report file %s: %w", filePath, err)
		}

		report, err := rs.unmarshalReport(data)
		if err != nil {
			return fmt.Errorf("unmarshal report file %s: %w", filePath, err)
		}

		if err := fn(report); err != nil {
			return err
		}
	}

	return nil
}

func (rs *LocalReportStore) writeIndex(dirPath string, builder *indexBuilder) error {
	indexPath := filepath.Join(dirPath, indexFileName)
	if builder.count == 0 {
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
		if closeErr := spill.Close(); closeErr != nil {
			slog.Error("failed to close spill after load error", "error", closeErr)
		}

		return nil, fmt.Errorf("load reports into spill: %w", err)
	}

	return spill, nil
}

type storedSourceState struct {
	source  m.Source
	mutator map[string]int
}

// CleanReports deletes stored report files that belong to the provided sources.
// It is safe to call when the reports directory does not exist.
func (rs *LocalReportStore) CleanReports(ctx context.Context, path m.Path, sources pkg.FileSpill[m.Source]) error {
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
	if len(toCleanPaths) == 0 && len(toCleanHashes) == 0 {
		return nil
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return fmt.Errorf("read reports directory: %w", err)
	}

	for _, entry := range entries {
		if err := rs.cleanReportFile(dirPath, entry, toCleanPaths, toCleanHashes); err != nil {
			return err
		}
	}

	return nil
}

func (rs *LocalReportStore) cleanReportFile(dirPath string, entry os.DirEntry, toCleanPaths map[string]bool, toCleanHashes map[string]bool) error {
	if !rs.shouldLoadReportEntry(entry) {
		return nil
	}

	filePath := filepath.Join(dirPath, entry.Name())
	// #nosec G304 -- filePath is built from a trusted reports directory listing
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read report file %s: %w", filePath, err)
	}

	report, err := rs.unmarshalReport(data)
	if err != nil {
		return fmt.Errorf("unmarshal report file %s: %w", filePath, err)
	}

	if !rs.shouldCleanReport(report.Source, toCleanPaths, toCleanHashes) {
		return nil
	}

	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove report file %s: %w", filePath, err)
	}

	return nil
}

func (rs *LocalReportStore) buildCleanMatchers(sources pkg.FileSpill[m.Source]) (map[string]bool, map[string]bool, error) {
	paths := make(map[string]bool)
	hashes := make(map[string]bool)

	if sources == nil {
		return paths, hashes, nil
	}

	err := sources.Range(func(_ uint64, src m.Source) error {
		if src.Origin == nil {
			return nil
		}

		if key := src.Key(); key != "" {
			paths[key] = true
		}

		if src.Origin.Hash != "" {
			hashes[src.Origin.Hash] = true
		}

		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("read sources to clean: %w", err)
	}

	return paths, hashes, nil
}

func (rs *LocalReportStore) shouldCleanReport(source m.Source, toCleanPaths map[string]bool, toCleanHashes map[string]bool) bool {
	if source.Origin == nil {
		return false
	}

	if key := source.Key(); key != "" && toCleanPaths[key] {
		return true
	}

	if source.Origin.Hash != "" {
		if toCleanHashes[source.Origin.Hash] {
			return true
		}
	}

	return false
}

// CheckUpdates splits sources into those that must be re-tested and the stored
// sources whose files no longer exist. A source is re-tested when it has no
// stored reports, its source or test hash changed, or the mutators that produced
// its reports changed. Both returned spills must be closed by the caller.
func (rs *LocalReportStore) CheckUpdates(ctx context.Context, path m.Path, sources pkg.FileSpill[m.Source]) (pkg.FileSpill[m.Source], pkg.FileSpill[m.Source], error) {
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

	return rs.splitChangedAndDeleted(stored, sources)
}

func (rs *LocalReportStore) splitChangedAndDeleted(stored map[string]storedSourceState, sources pkg.FileSpill[m.Source]) (pkg.FileSpill[m.Source], pkg.FileSpill[m.Source], error) {
	changed, err := pkg.NewFileSpill[m.Source]()
	if err != nil {
		return nil, nil, fmt.Errorf("create changed sources spill: %w", err)
	}

	deleted, err := pkg.NewFileSpill[m.Source]()
	if err != nil {
		return nil, nil, errors.Join(fmt.Errorf("create deleted sources spill: %w", err), changed.Close())
	}

	visited := make(map[string]bool)

	err = sources.Range(func(_ uint64, src m.Source) error {
		key := src.Key()
		if key != "" {
			visited[key] = true
		}

		if key == "" || rs.isSourceChanged(stored, key, src) {
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

func appendDeleted(stored map[string]storedSourceState, visited map[string]bool, deleted pkg.FileSpill[m.Source]) error {
	keys := make([]string, 0, len(stored))
	for key := range stored {
		if !visited[key] {
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

func (rs *LocalReportStore) isSourceChanged(stored map[string]storedSourceState, pathStr string, current m.Source) bool {
	st, ok := stored[pathStr]
	if !ok {
		return true
	}

	if rs.sourceHashChanged(st.source, current) {
		return true
	}

	return rs.mutatorsChanged(st.mutator)
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

func (rs *LocalReportStore) sourceHashChanged(stored m.Source, current m.Source) bool {
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
	current := currentMutationVersions()

	// Check if any mutator versions changed for mutators that were stored
	for name, storedVersion := range stored {
		// -1 indicates version mismatch across reports - needs re-run
		if storedVersion == -1 {
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

func currentMutationVersions() map[string]int {
	// Keep in sync with supported mutation types.
	mutations := []m.MutationType{
		m.MutationArithmetic,
		m.MutationBoolean,
		m.MutationNumbers,
		m.MutationComparison,
		m.MutationLogical,
		m.MutationUnary,
	}

	out := make(map[string]int, len(mutations))
	for _, mt := range mutations {
		out[mt.Name] = mt.Version
	}

	return out
}

func (rs *LocalReportStore) marshalReport(report m.Report) ([]byte, error) {
	encoded := reportYAML{
		Source: report.Source,
		Result: encodeResult(report.Result),
		Diff:   report.Diff,
	}

	return yaml.Marshal(encoded)
}

func (rs *LocalReportStore) unmarshalReport(data []byte) (m.Report, error) {
	var decoded reportYAML
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		return m.Report{}, err
	}

	return m.Report{
		Source: decoded.Source,
		Result: decodeResult(decoded.Result),
		Diff:   decoded.Diff,
	}, nil
}

func encodeResult(result m.Result) []resultEntryYAML {
	if len(result) == 0 {
		return []resultEntryYAML{}
	}

	keys := make([]m.MutationType, 0, len(result))
	for mutationType := range result {
		keys = append(keys, mutationType)
	}

	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Name != keys[j].Name {
			return keys[i].Name < keys[j].Name
		}

		return keys[i].Version < keys[j].Version
	})

	entries := make([]resultEntryYAML, 0, len(keys))
	for _, mutationType := range keys {
		results := result[mutationType]
		entry := resultEntryYAML{
			Name:      mutationType.Name,
			Version:   mutationType.Version,
			Mutations: make([]mutationResultYAML, 0, len(results)),
		}

		for _, res := range results {
			errString := ""
			if res.Err != nil {
				errString = res.Err.Error()
			}

			entry.Mutations = append(entry.Mutations, mutationResultYAML{
				MutationID: res.MutationID,
				Status:     res.Status,
				Err:        errString,
			})
		}

		entries = append(entries, entry)
	}

	return entries
}

func decodeResult(entries []resultEntryYAML) m.Result {
	if len(entries) == 0 {
		return m.Result{}
	}

	result := m.Result{}

	for _, entry := range entries {
		mutationType := m.MutationType{Name: entry.Name, Version: entry.Version}
		result[mutationType] = make([]struct {
			MutationID string
			Status     m.TestStatus
			Err        error
		}, 0, len(entry.Mutations))

		for _, mut := range entry.Mutations {
			result[mutationType] = append(result[mutationType], struct {
				MutationID string
				Status     m.TestStatus
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
	mutationsBySource map[string]map[string]*mutationEntry
}

type indexBuilder struct {
	rs    *LocalReportStore
	index indexEntry
	state indexState
	count int
}

func newIndexBuilder(rs *LocalReportStore) *indexBuilder {
	return &indexBuilder{
		rs:    rs,
		index: indexEntry{Result: make([]resultEntry, 0)},
		state: indexState{mutationsBySource: make(map[string]map[string]*mutationEntry)},
	}
}

func (b *indexBuilder) add(report m.Report) {
	b.count++

	sourceHex := b.rs.sourceHex(report.Source)
	if sourceHex != "" && b.state.mutationsBySource[sourceHex] == nil {
		b.state.mutationsBySource[sourceHex] = make(map[string]*mutationEntry)
	}

	reportHash := b.rs.computeReportHash(report.Result)
	if reportHash == "" {
		return
	}

	reportFile := reportHash + ".yaml"

	for mutationType, results := range report.Result {
		for _, result := range results {
			b.index.TotalMutations++
			b.rs.incrementStatusCount(&b.index, result.Status)
		}

		b.rs.trackMutationForIndex(&b.state, sourceHex, mutationType.Name, reportFile)
	}
}

func (b *indexBuilder) build() indexEntry {
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
		entries = make(map[string]*mutationEntry)
		state.mutationsBySource[sourceHex] = entries
	}

	entry, exists := entries[mutationName]
	if !exists {
		entry = &mutationEntry{MutationName: mutationName, MutationReports: []string{}}
		entries[mutationName] = entry
	}

	entry.MutationReports = append(entry.MutationReports, reportFile)
}

func (rs *LocalReportStore) buildIndexResults(state indexState) []resultEntry {
	sourceHexes := make([]string, 0, len(state.mutationsBySource))
	for sourceHex := range state.mutationsBySource {
		sourceHexes = append(sourceHexes, sourceHex)
	}

	sort.Strings(sourceHexes)

	results := make([]resultEntry, 0, len(sourceHexes))
	for _, sourceHex := range sourceHexes {
		entries := state.mutationsBySource[sourceHex]

		names := make([]string, 0, len(entries))
		for name := range entries {
			names = append(names, name)
		}

		sort.Strings(names)

		out := resultEntry{SourceHex: sourceHex, Mutations: make([]mutationEntry, 0, len(names))}
		for _, name := range names {
			out.Mutations = append(out.Mutations, *entries[name])
		}

		results = append(results, out)
	}

	return results
}

func (rs *LocalReportStore) sourceHex(source m.Source) string {
	if source.Origin == nil {
		return ""
	}

	return source.Origin.Hash
}

func (rs *LocalReportStore) incrementStatusCount(index *indexEntry, status m.TestStatus) {
	switch status {
	case m.Killed:
		index.KilledMutations++
	case m.Survived:
		index.SurvivedMutations++
	case m.Error, m.Timeout:
		index.FailedMutations++
	case m.Skipped:
		index.IgnoredMutations++
	case m.NotCovered:
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

func (rs *LocalReportStore) shouldLoadReportEntry(entry os.DirEntry) bool {
	if entry.IsDir() {
		return false
	}

	name := entry.Name()
	if name == indexFileName {
		return false
	}

	return strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")
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
