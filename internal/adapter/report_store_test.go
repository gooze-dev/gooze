package adapter

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	m "gooze.dev/pkg/gooze/internal/model"
	"gooze.dev/pkg/gooze/pkg"
)

func TestLocalReportStore_SaveReports_WritesHashedYAMLPerReport(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rs := &LocalReportStore{}

	errBoom := errors.New("boom")
	report := m.Report{
		Source: m.Source{
			Origin: &m.File{FullPath: m.Path("/abs/path/file.go"), Hash: "abc123"},
			Test:   &m.File{FullPath: m.Path("/abs/path/file_test.go"), Hash: "def456"},
		},
		Result: m.Result{
			m.MutationBoolean: {
				{MutationID: "m1", Status: m.Killed, Err: nil},
				{MutationID: "m2", Status: m.Error, Err: errBoom},
			},
			m.MutationArithmetic: {
				{MutationID: "m3", Status: m.Survived, Err: nil},
			},
		},
		Diff: nil,
	}

	expectedHash := rs.computeReportHash(report.Result)
	if expectedHash == "" {
		t.Fatalf("expected non-empty report hash")
	}

	if err := rs.SaveReports(m.Path(dir), []m.Report{report}); err != nil {
		t.Fatalf("SaveReports returned error: %v", err)
	}

	// Assert file exists and name matches expected hash.
	expectedFile := filepath.Join(dir, expectedHash+".yaml")
	info, err := os.Stat(expectedFile)
	if err != nil {
		t.Fatalf("expected report file %s to exist: %v", expectedFile, err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("expected %s to be a regular file", expectedFile)
	}

	// Basic shape check for filename (16 hex chars).
	matched, err := regexp.MatchString(`^[0-9a-f]{16}\.yaml$`, filepath.Base(expectedFile))
	if err != nil {
		t.Fatalf("regex error: %v", err)
	}
	if !matched {
		t.Fatalf("unexpected filename: %s", filepath.Base(expectedFile))
	}

	// Decode YAML and validate structure.
	data, err := os.ReadFile(expectedFile)
	if err != nil {
		t.Fatalf("read report file: %v", err)
	}

	var decoded reportYAML
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal YAML: %v", err)
	}

	if decoded.Source.Origin == nil || decoded.Source.Test == nil {
		t.Fatalf("expected source origin and test to be present")
	}
	if decoded.Source.Origin.Hash != "abc123" {
		t.Fatalf("unexpected origin hash: %s", decoded.Source.Origin.Hash)
	}
	if decoded.Diff != nil {
		t.Fatalf("expected diff to be nil")
	}

	if len(decoded.Result) != 2 {
		t.Fatalf("expected 2 result entries, got %d", len(decoded.Result))
	}

	// Ensure the boolean mutation entry includes the error string for m2.
	foundM2Err := false
	for _, entry := range decoded.Result {
		if entry.Name != m.MutationBoolean.Name {
			continue
		}
		for _, mut := range entry.Mutations {
			if mut.MutationID == "m2" {
				foundM2Err = true
				if mut.Err != "boom" {
					t.Fatalf("expected m2 err to be 'boom', got %q", mut.Err)
				}
			}
		}
	}
	if !foundM2Err {
		t.Fatalf("expected to find mutation m2 in decoded YAML")
	}
}

func TestLocalReportStore_SaveReports_SkipsReportsWithNoMutations(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rs := &LocalReportStore{}

	report := m.Report{
		Source: m.Source{Origin: &m.File{FullPath: m.Path("/abs/path/file.go"), Hash: "abc123"}},
		Result: m.Result{},
		Diff:   nil,
	}

	if err := rs.SaveReports(m.Path(dir), []m.Report{report}); err != nil {
		t.Fatalf("SaveReports returned error: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir returned error: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no report files to be written, found %d", len(entries))
	}
}

func TestLocalReportStore_SaveReports_WritesIndexYAML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rs := &LocalReportStore{}

	report1 := m.Report{
		Source: m.Source{Origin: &m.File{FullPath: m.Path("/abs/a.go"), Hash: "sourceA"}},
		Result: m.Result{
			m.MutationBoolean: {
				{MutationID: "b1", Status: m.Killed, Err: nil},
				{MutationID: "b2", Status: m.Skipped, Err: nil},
			},
		},
	}

	report2 := m.Report{
		Source: m.Source{Origin: &m.File{FullPath: m.Path("/abs/b.go"), Hash: "sourceB"}},
		Result: m.Result{
			m.MutationArithmetic: {
				{MutationID: "a1", Status: m.Error, Err: errors.New("nope")},
			},
		},
	}

	if err := rs.SaveReports(m.Path(dir), []m.Report{report1, report2}); err != nil {
		t.Fatalf("SaveReports returned error: %v", err)
	}

	indexPath := filepath.Join(dir, "_index.yaml")
	if _, err := os.Stat(indexPath); err == nil {
		t.Fatalf("expected _index.yaml to not exist until RegenerateIndex is called")
	}

	if err := rs.RegenerateIndex(m.Path(dir)); err != nil {
		t.Fatalf("RegenerateIndex returned error: %v", err)
	}
	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("expected _index.yaml to exist: %v", err)
	}

	var idx indexEntry
	if err := yaml.Unmarshal(data, &idx); err != nil {
		t.Fatalf("unmarshal _index.yaml: %v", err)
	}

	if idx.TotalMutations != 3 {
		t.Fatalf("expected total_mutations=3, got %d", idx.TotalMutations)
	}
	if idx.KilledMutations != 1 {
		t.Fatalf("expected killed_mutations=1, got %d", idx.KilledMutations)
	}
	if idx.IgnoredMutations != 1 {
		t.Fatalf("expected ignored_mutations=1, got %d", idx.IgnoredMutations)
	}
	if idx.FailedMutations != 1 {
		t.Fatalf("expected failed_mutations=1, got %d", idx.FailedMutations)
	}
	if idx.SurvivedMutations != 0 {
		t.Fatalf("expected survived_mutations=0, got %d", idx.SurvivedMutations)
	}

	if len(idx.Result) != 2 {
		t.Fatalf("expected 2 result entries, got %d", len(idx.Result))
	}

	hash1 := rs.computeReportHash(report1.Result)
	hash2 := rs.computeReportHash(report2.Result)
	if hash1 == "" || hash2 == "" {
		t.Fatalf("expected non-empty report hashes")
	}
	file1 := hash1 + ".yaml"
	file2 := hash2 + ".yaml"

	bySource := map[string]resultEntry{}
	for _, re := range idx.Result {
		bySource[re.SourceHex] = re
	}

	reA, ok := bySource["sourceA"]
	if !ok {
		t.Fatalf("missing sourceA entry")
	}
	if len(reA.Mutations) != 1 {
		t.Fatalf("expected sourceA to have 1 mutation entry, got %d", len(reA.Mutations))
	}
	if reA.Mutations[0].MutationName != m.MutationBoolean.Name {
		t.Fatalf("expected sourceA mutation_name=%q, got %q", m.MutationBoolean.Name, reA.Mutations[0].MutationName)
	}
	if len(reA.Mutations[0].MutationReports) != 1 || reA.Mutations[0].MutationReports[0] != file1 {
		t.Fatalf("unexpected sourceA mutation_reports: %v", reA.Mutations[0].MutationReports)
	}

	reB, ok := bySource["sourceB"]
	if !ok {
		t.Fatalf("missing sourceB entry")
	}
	if len(reB.Mutations) != 1 {
		t.Fatalf("expected sourceB to have 1 mutation entry, got %d", len(reB.Mutations))
	}
	if reB.Mutations[0].MutationName != m.MutationArithmetic.Name {
		t.Fatalf("expected sourceB mutation_name=%q, got %q", m.MutationArithmetic.Name, reB.Mutations[0].MutationName)
	}
	if len(reB.Mutations[0].MutationReports) != 1 || reB.Mutations[0].MutationReports[0] != file2 {
		t.Fatalf("unexpected sourceB mutation_reports: %v", reB.Mutations[0].MutationReports)
	}
}

func TestLocalReportStore_SaveSpillReports_WritesHashedYAMLPerReport(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rs := &LocalReportStore{}

	// Create a spill and populate it
	spill, err := pkg.NewFileSpill[m.Report]()
	if err != nil {
		t.Fatalf("failed to create filespill: %v", err)
	}
	defer spill.Close()

	report := m.Report{
		Source: m.Source{
			Origin: &m.File{FullPath: m.Path("/abs/path/file.go"), Hash: "abc123"},
			Test:   &m.File{FullPath: m.Path("/abs/path/file_test.go"), Hash: "def456"},
		},
		Result: m.Result{
			m.MutationBoolean: {
				{MutationID: "m1", Status: m.Killed, Err: nil},
			},
		},
	}

	err = spill.Append(report)
	if err != nil {
		t.Fatalf("failed to append to filespill: %v", err)
	}

	expectedHash := rs.computeReportHash(report.Result)

	// Invoke method under test
	if err := rs.SaveSpillReports(m.Path(dir), spill); err != nil {
		t.Fatalf("SaveSpillReports returned error: %v", err)
	}

	// Verify file exists
	expectedPath := filepath.Join(dir, expectedHash+".yaml")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Fatalf("expected report file to be written at %s", expectedPath)
	}
}

func TestLocalReportStore_LoadSpillReports_LoadsReportsIntoSpill(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rs := &LocalReportStore{}

	// First, save some reports using SaveReports
	report1 := m.Report{
		Source: m.Source{
			Origin: &m.File{FullPath: m.Path("/abs/path/file1.go"), Hash: "abc123"},
			Test:   &m.File{FullPath: m.Path("/abs/path/file1_test.go"), Hash: "def456"},
		},
		Result: m.Result{
			m.MutationBoolean: {
				{MutationID: "m1", Status: m.Killed, Err: nil},
			},
		},
	}
	report2 := m.Report{
		Source: m.Source{
			Origin: &m.File{FullPath: m.Path("/abs/path/file2.go"), Hash: "ghi789"},
			Test:   &m.File{FullPath: m.Path("/abs/path/file2_test.go"), Hash: "jkl012"},
		},
		Result: m.Result{
			m.MutationArithmetic: {
				{MutationID: "m2", Status: m.Survived, Err: nil},
			},
		},
	}

	if err := rs.SaveReports(m.Path(dir), []m.Report{report1, report2}); err != nil {
		t.Fatalf("SaveReports returned error: %v", err)
	}

	// Now load them back into a spill
	loadedSpill, err := rs.LoadSpillReports(m.Path(dir))
	if err != nil {
		t.Fatalf("LoadSpillReports returned error: %v", err)
	}
	defer loadedSpill.Close()

	// Collect loaded reports
	var loadedReports []m.Report
	err = loadedSpill.Range(func(_ uint64, report m.Report) error {
		loadedReports = append(loadedReports, report)
		return nil
	})
	if err != nil {
		t.Fatalf("Range on loaded spill returned error: %v", err)
	}

	// Verify we have the expected number of reports
	if len(loadedReports) != 2 {
		t.Fatalf("expected 2 reports, got %d", len(loadedReports))
	}

	// For simplicity, check that the sources are present (order may vary)
	sources := make(map[string]bool)
	for _, r := range loadedReports {
		sources[string(r.Source.Origin.FullPath)] = true
	}
	if !sources["/abs/path/file1.go"] || !sources["/abs/path/file2.go"] {
		t.Fatalf("loaded reports do not match expected sources")
	}
}

func TestLocalReportStore_CheckUpdates_NoReportsDir_ReturnsAllSources(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "does-not-exist")
	rs := &LocalReportStore{}

	sources := []m.Source{
		{Origin: &m.File{FullPath: m.Path("/abs/a.go"), Hash: "hash-a"}},
		{Origin: &m.File{FullPath: m.Path("/abs/b.go"), Hash: "hash-b"}},
	}

	changed, err := rs.CheckUpdates(m.Path(dir), sources)
	if err != nil {
		t.Fatalf("CheckUpdates returned error: %v", err)
	}
	if !reflect.DeepEqual(changed, sources) {
		t.Fatalf("changed sources = %#v, want %#v", changed, sources)
	}
}

func TestLocalReportStore_CheckUpdates_EmptyPath_ReturnsError(t *testing.T) {
	t.Parallel()

	rs := &LocalReportStore{}
	_, err := rs.CheckUpdates("", nil)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "reports directory path is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLocalReportStore_CheckUpdates_PathIsFile_ReturnsError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(filePath, []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	rs := &LocalReportStore{}
	_, err := rs.CheckUpdates(m.Path(filePath), nil)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "path is not a directory") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLocalReportStore_CheckUpdates_EmptyReportsDir_ReturnsAllSources(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rs := &LocalReportStore{}

	sources := []m.Source{{Origin: &m.File{FullPath: m.Path("/abs/a.go"), Hash: "hash-a"}}}
	changed, err := rs.CheckUpdates(m.Path(dir), sources)
	if err != nil {
		t.Fatalf("CheckUpdates returned error: %v", err)
	}
	if !reflect.DeepEqual(changed, sources) {
		t.Fatalf("changed sources = %#v, want %#v", changed, sources)
	}
}

func TestLocalReportStore_CheckUpdates_SourceDeleted_ReturnsMissingSource(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rs := &LocalReportStore{}

	deleted := m.Source{Origin: &m.File{FullPath: m.Path("/abs/deleted.go"), Hash: "old-hash"}}
	report := m.Report{
		Source: deleted,
		Result: m.Result{m.MutationBoolean: {{MutationID: "m1", Status: m.Killed, Err: nil}}},
	}
	if err := rs.SaveReports(m.Path(dir), []m.Report{report}); err != nil {
		t.Fatalf("SaveReports returned error: %v", err)
	}

	changed, err := rs.CheckUpdates(m.Path(dir), nil)
	if err != nil {
		t.Fatalf("CheckUpdates returned error: %v", err)
	}
	if len(changed) != 1 {
		t.Fatalf("expected 1 changed source, got %d", len(changed))
	}
	if changed[0].Origin == nil || changed[0].Origin.FullPath != deleted.Origin.FullPath {
		t.Fatalf("expected deleted source %q, got %#v", deleted.Origin.FullPath, changed[0])
	}
}

func TestLocalReportStore_CheckUpdates_CodeOrTestChanged_ReturnsSource(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rs := &LocalReportStore{}

	old := m.Source{
		Origin: &m.File{FullPath: m.Path("/abs/a.go"), Hash: "old-code"},
		Test:   &m.File{FullPath: m.Path("/abs/a_test.go"), Hash: "old-test"},
	}
	report := m.Report{
		Source: old,
		Result: m.Result{m.MutationBoolean: {{MutationID: "m1", Status: m.Killed, Err: nil}}},
	}
	if err := rs.SaveReports(m.Path(dir), []m.Report{report}); err != nil {
		t.Fatalf("SaveReports returned error: %v", err)
	}

	current := []m.Source{{
		Origin: &m.File{FullPath: m.Path("/abs/a.go"), Hash: "new-code"},
		Test:   &m.File{FullPath: m.Path("/abs/a_test.go"), Hash: "old-test"},
	}}

	changed, err := rs.CheckUpdates(m.Path(dir), current)
	if err != nil {
		t.Fatalf("CheckUpdates returned error: %v", err)
	}
	if len(changed) != 1 {
		t.Fatalf("expected 1 changed source, got %d", len(changed))
	}
	if changed[0].Origin == nil || changed[0].Origin.Hash != "new-code" {
		t.Fatalf("expected returned source to be current, got %#v", changed[0])
	}
}

func TestLocalReportStore_CheckUpdates_TestFileAddedOrRemoved_ReturnsSource(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rs := &LocalReportStore{}

	// Stored run had a test file.
	stored := m.Source{
		Origin: &m.File{FullPath: m.Path("/abs/a.go"), Hash: "same"},
		Test:   &m.File{FullPath: m.Path("/abs/a_test.go"), Hash: "test-hash"},
	}
	report := m.Report{
		Source: stored,
		Result: m.Result{m.MutationBoolean: {{MutationID: "m1", Status: m.Killed, Err: nil}}},
	}
	if err := rs.SaveReports(m.Path(dir), []m.Report{report}); err != nil {
		t.Fatalf("SaveReports returned error: %v", err)
	}

	// Current run has no test file associated.
	current := []m.Source{{Origin: &m.File{FullPath: m.Path("/abs/a.go"), Hash: "same"}}}
	changed, err := rs.CheckUpdates(m.Path(dir), current)
	if err != nil {
		t.Fatalf("CheckUpdates returned error: %v", err)
	}
	if len(changed) != 1 {
		t.Fatalf("expected 1 changed source, got %d", len(changed))
	}
}

func TestLocalReportStore_CheckUpdates_NewMutator_ReturnsSource(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rs := &LocalReportStore{}

	old := m.Source{Origin: &m.File{FullPath: m.Path("/abs/a.go"), Hash: "same"}}

	// Previous run only had boolean mutations recorded.
	report := m.Report{
		Source: old,
		Result: m.Result{m.MutationBoolean: {{MutationID: "m1", Status: m.Killed, Err: nil}}},
	}
	if err := rs.SaveReports(m.Path(dir), []m.Report{report}); err != nil {
		t.Fatalf("SaveReports returned error: %v", err)
	}

	current := []m.Source{{Origin: &m.File{FullPath: m.Path("/abs/a.go"), Hash: "same"}}}
	changed, err := rs.CheckUpdates(m.Path(dir), current)
	if err != nil {
		t.Fatalf("CheckUpdates returned error: %v", err)
	}
	if len(changed) != 0 {
		t.Fatalf("expected 0 changed sources (new mutators don't trigger re-test), got %d", len(changed))
	}
}

func TestLocalReportStore_CheckUpdates_StoredHasUnknownMutator_ReturnsSource(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rs := &LocalReportStore{}

	old := m.Source{Origin: &m.File{FullPath: m.Path("/abs/a.go"), Hash: "same"}}
	unknown := m.MutationType{Name: "custom-mut", Version: 1}
	report := m.Report{
		Source: old,
		Result: m.Result{unknown: {{MutationID: "m1", Status: m.Killed, Err: nil}}},
	}
	if err := rs.SaveReports(m.Path(dir), []m.Report{report}); err != nil {
		t.Fatalf("SaveReports returned error: %v", err)
	}

	current := []m.Source{{Origin: &m.File{FullPath: m.Path("/abs/a.go"), Hash: "same"}}}
	changed, err := rs.CheckUpdates(m.Path(dir), current)
	if err != nil {
		t.Fatalf("CheckUpdates returned error: %v", err)
	}
	if len(changed) != 1 {
		t.Fatalf("expected 1 changed source due to removed/unknown mutator, got %d", len(changed))
	}
}

func TestLocalReportStore_CheckUpdates_IgnoresSourcesWithNilOrigin(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "does-not-exist")
	rs := &LocalReportStore{}

	sources := []m.Source{{Origin: nil}}
	changed, err := rs.CheckUpdates(m.Path(dir), sources)
	if err != nil {
		t.Fatalf("CheckUpdates returned error: %v", err)
	}
	if len(changed) != 1 {
		t.Fatalf("expected passthrough (no reports) to return all sources, got %d", len(changed))
	}
}

func TestLocalReportStore_CleanReports_DeletesOnlySelectedAndRegeneratesIndex(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rs := &LocalReportStore{}

	sourceA := m.Source{Origin: &m.File{FullPath: m.Path("/abs/a.go"), Hash: "sourceA"}}
	sourceB := m.Source{Origin: &m.File{FullPath: m.Path("/abs/b.go"), Hash: "sourceB"}}

	reportA := m.Report{Source: sourceA, Result: m.Result{m.MutationBoolean: {{MutationID: "b1", Status: m.Killed, Err: nil}}}}
	reportB := m.Report{Source: sourceB, Result: m.Result{m.MutationArithmetic: {{MutationID: "a1", Status: m.Survived, Err: nil}}}}

	if err := rs.SaveReports(m.Path(dir), []m.Report{reportA, reportB}); err != nil {
		t.Fatalf("SaveReports returned error: %v", err)
	}
	if err := rs.RegenerateIndex(m.Path(dir)); err != nil {
		t.Fatalf("RegenerateIndex returned error: %v", err)
	}

	hashA := rs.computeReportHash(reportA.Result)
	hashB := rs.computeReportHash(reportB.Result)
	fileA := filepath.Join(dir, hashA+".yaml")
	fileB := filepath.Join(dir, hashB+".yaml")

	if _, err := os.Stat(fileA); err != nil {
		t.Fatalf("expected report A file to exist: %v", err)
	}
	if _, err := os.Stat(fileB); err != nil {
		t.Fatalf("expected report B file to exist: %v", err)
	}
	indexPath := filepath.Join(dir, "_index.yaml")
	if _, err := os.Stat(indexPath); err != nil {
		t.Fatalf("expected _index.yaml to exist: %v", err)
	}

	if err := rs.CleanReports(m.Path(dir), []m.Source{sourceA}); err != nil {
		t.Fatalf("CleanReports returned error: %v", err)
	}

	if _, err := os.Stat(fileA); err == nil {
		t.Fatalf("expected report A file to be deleted")
	}
	if _, err := os.Stat(fileB); err != nil {
		t.Fatalf("expected report B file to remain: %v", err)
	}

	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read _index.yaml: %v", err)
	}
	var idx indexEntry
	if err := yaml.Unmarshal(data, &idx); err != nil {
		t.Fatalf("unmarshal _index.yaml: %v", err)
	}
	if len(idx.Result) != 1 {
		t.Fatalf("expected 1 result entry after cleaning, got %d", len(idx.Result))
	}
	if idx.Result[0].SourceHex != "sourceB" {
		t.Fatalf("expected remaining source to be sourceB, got %q", idx.Result[0].SourceHex)
	}
}

func TestLocalReportStore_CleanReports_DeleteAll_RemovesIndex(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rs := &LocalReportStore{}

	sourceA := m.Source{Origin: &m.File{FullPath: m.Path("/abs/a.go"), Hash: "sourceA"}}
	reportA := m.Report{Source: sourceA, Result: m.Result{m.MutationBoolean: {{MutationID: "b1", Status: m.Killed, Err: nil}}}}

	if err := rs.SaveReports(m.Path(dir), []m.Report{reportA}); err != nil {
		t.Fatalf("SaveReports returned error: %v", err)
	}
	if err := rs.RegenerateIndex(m.Path(dir)); err != nil {
		t.Fatalf("RegenerateIndex returned error: %v", err)
	}

	hashA := rs.computeReportHash(reportA.Result)
	fileA := filepath.Join(dir, hashA+".yaml")
	indexPath := filepath.Join(dir, "_index.yaml")
	if _, err := os.Stat(fileA); err != nil {
		t.Fatalf("expected report file to exist: %v", err)
	}
	if _, err := os.Stat(indexPath); err != nil {
		t.Fatalf("expected _index.yaml to exist: %v", err)
	}

	if err := rs.CleanReports(m.Path(dir), []m.Source{sourceA}); err != nil {
		t.Fatalf("CleanReports returned error: %v", err)
	}

	if _, err := os.Stat(fileA); err == nil {
		t.Fatalf("expected report file to be deleted")
	}
	if _, err := os.Stat(indexPath); err == nil {
		t.Fatalf("expected _index.yaml to be deleted")
	}
}

func TestLocalReportStore_CleanReports_NoReportsDir_NoError(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "does-not-exist")
	rs := &LocalReportStore{}

	if err := rs.CleanReports(m.Path(dir), nil); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestLocalReportStore_CleanReports_EmptyPath_ReturnsError(t *testing.T) {
	t.Parallel()

	rs := &LocalReportStore{}
	if err := rs.CleanReports("", nil); err == nil {
		t.Fatalf("expected error")
	}
}

func TestLocalReportStore_CheckUpdates_MutatorVersionDiff_ReturnsSource(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rs := &LocalReportStore{}

	old := m.Source{Origin: &m.File{FullPath: m.Path("/abs/a.go"), Hash: "same"}}

	// Simulate a stored report with an older boolean mutator version.
	oldBool := m.MutationType{Name: m.MutationBoolean.Name, Version: 0}
	report := m.Report{
		Source: old,
		Result: m.Result{oldBool: {{MutationID: "m1", Status: m.Killed, Err: nil}}},
	}
	if err := rs.SaveReports(m.Path(dir), []m.Report{report}); err != nil {
		t.Fatalf("SaveReports returned error: %v", err)
	}

	current := []m.Source{{Origin: &m.File{FullPath: m.Path("/abs/a.go"), Hash: "same"}}}
	changed, err := rs.CheckUpdates(m.Path(dir), current)
	if err != nil {
		t.Fatalf("CheckUpdates returned error: %v", err)
	}
	if len(changed) != 1 {
		t.Fatalf("expected 1 changed source due to mutator version diff, got %d", len(changed))
	}
}
