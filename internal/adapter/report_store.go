// Package adapter contains UI and infrastructure adapters for the Gooze CLI.
package adapter

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	m "github.com/mouse-blink/gooze/internal/model"
)

// ReportStore persists and retrieves mutation reports.
type ReportStore interface {
	SaveReports(path m.Path, reports []m.Report) error
	RegenerateIndex(path m.Path) error
	LoadReports(path m.Path) ([]m.Report, error)
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

// RegenerateIndex rebuilds and writes `index.yaml` from the report files in `path`.
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
	indexPath := filepath.Join(dirPath, "index.yaml")
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
func (rs *LocalReportStore) LoadReports(_ m.Path) ([]m.Report, error) {
	return nil, nil
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
	case m.Error:
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
	if name == "index.yaml" {
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
