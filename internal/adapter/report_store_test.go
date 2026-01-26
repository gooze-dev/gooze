package adapter

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"gopkg.in/yaml.v3"

	m "github.com/mouse-blink/gooze/internal/model"
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

	indexPath := filepath.Join(dir, "index.yaml")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("expected index.yaml to exist: %v", err)
	}

	var idx indexEntry
	if err := yaml.Unmarshal(data, &idx); err != nil {
		t.Fatalf("unmarshal index.yaml: %v", err)
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
