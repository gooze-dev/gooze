// Package adapter contains UI and infrastructure adapters for the Gooze CLI.
package adapter

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	m "gooze.dev/pkg/gooze/internal/model"
	"gooze.dev/pkg/gooze/pkg"
)

const indexFileName = "_index.yaml"

// ReportStore persists and retrieves mutation reports.
type ReportStore interface {
	SaveReports(path m.Path, reports []m.Report) error
	SaveSpillReports(path m.Path, reports pkg.FileSpill[m.Report]) error
	RegenerateIndex(path m.Path) error
	LoadReports(path m.Path) ([]m.Report, error)
	LoadSpillReports(path m.Path) (pkg.FileSpill[m.Report], error)
	CheckUpdates(path m.Path, sources []m.Source) ([]m.Source, error)
	CleanReports(path m.Path, sources []m.Source) error
}

// LocalReportStore is the concrete implementation that will back the
// ReportStore interface. It currently returns nil for LoadReports so tests
// can drive the actual logic.
type LocalReportStore struct{}

// NewReportStore constructs a LocalReportStore instance ready to
// be wired into the workflow.
func NewReportStore() ReportStore {
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
	TotalMutations    int           `yaml:"total_mutations"`
	KilledMutations   int           `yaml:"killed_mutations"`
	SurvivedMutations int           `yaml:"survived_mutations"`
	FailedMutations   int           `yaml:"failed_mutations"`
	IgnoredMutations  int           `yaml:"ignored_mutations"`
	Result            []resultEntry `yaml:"result"`
}

// SaveReports writes one YAML file per report into the provided directory.
func (rs *LocalReportStore) SaveReports(path m.Path, reports []m.Report) error {
	dirPath := string(path)
	if dirPath == "" {
		return fmt.Errorf("reports directory path is required")
	}

	if err := os.MkdirAll(dirPath, 0o750); err != nil {
		return fmt.Errorf("create reports directory: %w", err)
	}

	writtenReports := make([]m.Report, 0, len(reports))
	for _, report := range reports {
		reportHash := rs.computeReportHash(report.Result)
		if reportHash == "" {
			continue
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

		writtenReports = append(writtenReports, report)
	}

	if len(writtenReports) == 0 {
		return nil
	}

	return nil
}

// SaveSpillReports writes one YAML file per report from the filespill into the provided directory.
func (rs *LocalReportStore) SaveSpillReports(path m.Path, reports pkg.FileSpill[m.Report]) error {
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
func (rs *LocalReportStore) RegenerateIndex(path m.Path) error {
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

	reports, err := rs.loadReportsFromDir(dirPath)
	if err != nil {
		return err
	}

	return rs.writeIndexForReports(dirPath, reports)
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

func (rs *LocalReportStore) loadReportsFromDir(dirPath string) ([]m.Report, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("read reports directory: %w", err)
	}

	reports := make([]m.Report, 0)

	for _, entry := range entries {
		if !rs.shouldLoadReportEntry(entry) {
			continue
		}

		filePath := filepath.Join(dirPath, entry.Name())
		// #nosec G304 -- filePath is built from a trusted reports directory listing
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("read report file %s: %w", filePath, err)
		}

		report, err := rs.unmarshalReport(data)
		if err != nil {
			return nil, fmt.Errorf("unmarshal report file %s: %w", filePath, err)
		}

		reports = append(reports, report)
	}

	return reports, nil
}

func (rs *LocalReportStore) writeIndexForReports(dirPath string, reports []m.Report) error {
	indexPath := filepath.Join(dirPath, indexFileName)
	if len(reports) == 0 {
		_ = os.Remove(indexPath)
		return nil
	}

	index := rs.buildIndexFromReports(reports)

	indexData, err := yaml.Marshal(index)
	if err != nil {
		return fmt.Errorf("marshal index YAML: %w", err)
	}

	if err := os.WriteFile(indexPath, indexData, 0o600); err != nil {
		return fmt.Errorf("write index file %s: %w", indexPath, err)
	}

	return nil
}

// LoadReports retrieves previously saved reports from disk.
//
// Note: This is currently a stub; report loading will be implemented later.
func (rs *LocalReportStore) LoadReports(path m.Path) ([]m.Report, error) {
	dirPath := string(path)
	if dirPath == "" {
		return nil, fmt.Errorf("reports directory path is required")
	}

	if err := rs.validateReportsDir(dirPath); err != nil {
		return nil, err
	}

	reports, err := rs.loadReportsFromDir(dirPath)
	if err != nil {
		return nil, err
	}

	return reports, nil
}

// LoadSpillReports loads reports from a FileSpill (stubbed for now - assuming index logic handles it).
// In a real implementation this would likely return a FileSpill that reads directly from the persisted gob files.
// For now, if we saved as spilled, we might want to just open it.
func (rs *LocalReportStore) LoadSpillReports(path m.Path) (pkg.FileSpill[m.Report], error) {
	dirPath := string(path)
	if dirPath == "" {
		return nil, fmt.Errorf("reports directory path is required")
	}

	if err := rs.validateReportsDir(dirPath); err != nil {
		return nil, err
	}

	reports, err := rs.loadReportsFromDir(dirPath)
	if err != nil {
		return nil, err
	}

	spill, err := pkg.NewFileSpill[m.Report]()
	if err != nil {
		return nil, fmt.Errorf("create filespill: %w", err)
	}

	for _, report := range reports {
		if err := spill.Append(report); err != nil {
			if closeErr := spill.Close(); closeErr != nil {
				slog.Error("failed to close spill after append error", "error", closeErr)
			}

			return nil, fmt.Errorf("append report to spill: %w", err)
		}
	}

	return spill, nil
}

type storedSourceState struct {
	source  m.Source
	mutator map[string]int
}

// CleanReports deletes stored report files that belong to the provided sources.
// It is safe to call when the reports directory does not exist.
func (rs *LocalReportStore) CleanReports(path m.Path, sources []m.Source) error {
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

	toCleanPaths, toCleanHashes := rs.buildCleanMatchers(sources)
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

	return rs.RegenerateIndex(path)
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

func (rs *LocalReportStore) buildCleanMatchers(sources []m.Source) (map[string]bool, map[string]bool) {
	paths := make(map[string]bool)
	hashes := make(map[string]bool)

	for _, src := range sources {
		if src.Origin == nil {
			continue
		}

		if src.Origin.FullPath != "" {
			paths[string(src.Origin.FullPath)] = true
		}

		if src.Origin.Hash != "" {
			hashes[src.Origin.Hash] = true
		}
	}

	return paths, hashes
}

func (rs *LocalReportStore) shouldCleanReport(source m.Source, toCleanPaths map[string]bool, toCleanHashes map[string]bool) bool {
	if source.Origin == nil {
		return false
	}

	if source.Origin.FullPath != "" {
		if toCleanPaths[string(source.Origin.FullPath)] {
			return true
		}
	}

	if source.Origin.Hash != "" {
		if toCleanHashes[source.Origin.Hash] {
			return true
		}
	}

	return false
}

// CheckUpdates returns sources that should be re-tested because:
// - the source file is deleted (present in stored reports but not in current `sources`)
// - source/test content hash changed
// - the current mutator set or versions differ from what was used to generate stored reports.
func (rs *LocalReportStore) CheckUpdates(path m.Path, sources []m.Source) ([]m.Source, error) {
	dirPath := string(path)
	if dirPath == "" {
		return nil, fmt.Errorf("reports directory path is required")
	}

	exists, err := rs.reportsDirExists(dirPath)
	if err != nil {
		return nil, err
	}

	if !exists {
		// No prior reports: everything is effectively changed.
		return sources, nil
	}

	reports, err := rs.loadReportsFromDir(dirPath)
	if err != nil {
		return nil, err
	}

	stored := rs.buildStoredSourceState(reports)
	currentByPath := rs.buildCurrentSourceMap(sources)
	changed := rs.findChangedSources(stored, currentByPath)

	return changed, nil
}

func (rs *LocalReportStore) buildCurrentSourceMap(sources []m.Source) map[string]m.Source {
	currentByPath := map[string]m.Source{}

	for _, src := range sources {
		if src.Origin == nil || src.Origin.FullPath == "" {
			continue
		}

		currentByPath[string(src.Origin.FullPath)] = src
	}

	return currentByPath
}

func (rs *LocalReportStore) findChangedSources(stored map[string]storedSourceState, currentByPath map[string]m.Source) []m.Source {
	changed := make([]m.Source, 0)
	visited := make(map[string]bool, len(currentByPath))

	for pathStr, current := range currentByPath {
		visited[pathStr] = true
		if rs.isSourceChanged(stored, pathStr, current) {
			changed = append(changed, current)
		}
	}

	// Find deleted sources
	for pathStr, st := range stored {
		if !visited[pathStr] {
			changed = append(changed, st.source)
		}
	}

	return changed
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

func (rs *LocalReportStore) buildStoredSourceState(reports []m.Report) map[string]storedSourceState {
	state := make(map[string]storedSourceState)

	for _, report := range reports {
		if report.Source.Origin == nil || report.Source.Origin.FullPath == "" {
			continue
		}

		key := string(report.Source.Origin.FullPath)

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

	return state
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

func (rs *LocalReportStore) buildIndexFromReports(reports []m.Report) indexEntry {
	index := indexEntry{Result: make([]resultEntry, 0)}
	state := rs.collectIndexState(reports, &index)
	index.Result = rs.buildIndexResults(state)

	return index
}

type indexState struct {
	globalMutationMap map[string]*mutationEntry
	sourceToMutations map[string]map[string]bool
}

func (rs *LocalReportStore) collectIndexState(reports []m.Report, index *indexEntry) indexState {
	state := indexState{
		globalMutationMap: make(map[string]*mutationEntry),
		sourceToMutations: make(map[string]map[string]bool),
	}

	for _, report := range reports {
		sourceHex := rs.sourceHex(report.Source)
		if sourceHex != "" && state.sourceToMutations[sourceHex] == nil {
			state.sourceToMutations[sourceHex] = make(map[string]bool)
		}

		reportHash := rs.computeReportHash(report.Result)
		if reportHash == "" {
			continue
		}

		reportFile := reportHash + ".yaml"

		for mutationType, results := range report.Result {
			for _, result := range results {
				index.TotalMutations++
				rs.incrementStatusCount(index, result.Status)
			}

			rs.trackMutationForIndex(&state, sourceHex, mutationType.Name, reportFile)
		}
	}

	// Stabilize order.
	for _, ent := range state.globalMutationMap {
		sort.Strings(ent.MutationReports)
	}

	return state
}

func (rs *LocalReportStore) trackMutationForIndex(state *indexState, sourceHex string, mutationName string, reportFile string) {
	if sourceHex == "" {
		return
	}

	if state.sourceToMutations[sourceHex] == nil {
		state.sourceToMutations[sourceHex] = make(map[string]bool)
	}

	state.sourceToMutations[sourceHex][mutationName] = true

	if _, exists := state.globalMutationMap[mutationName]; !exists {
		state.globalMutationMap[mutationName] = &mutationEntry{MutationName: mutationName, MutationReports: []string{}}
	}

	if !rs.reportFileExists(state.globalMutationMap[mutationName].MutationReports, reportFile) {
		state.globalMutationMap[mutationName].MutationReports = append(state.globalMutationMap[mutationName].MutationReports, reportFile)
	}
}

func (rs *LocalReportStore) buildIndexResults(state indexState) []resultEntry {
	sourceHexes := make([]string, 0, len(state.sourceToMutations))
	for sourceHex := range state.sourceToMutations {
		sourceHexes = append(sourceHexes, sourceHex)
	}

	sort.Strings(sourceHexes)

	results := make([]resultEntry, 0, len(sourceHexes))
	for _, sourceHex := range sourceHexes {
		mutationNames := state.sourceToMutations[sourceHex]

		names := make([]string, 0, len(mutationNames))
		for name := range mutationNames {
			names = append(names, name)
		}

		sort.Strings(names)

		out := resultEntry{SourceHex: sourceHex, Mutations: make([]mutationEntry, 0, len(names))}
		for _, name := range names {
			if state.globalMutationMap[name] == nil {
				continue
			}

			out.Mutations = append(out.Mutations, *state.globalMutationMap[name])
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
	}
}

func (rs *LocalReportStore) reportFileExists(files []string, reportFile string) bool {
	for _, file := range files {
		if file == reportFile {
			return true
		}
	}

	return false
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
