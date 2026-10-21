package adapter

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	m "gooze.dev/pkg/gooze/internal/model"
	"gooze.dev/pkg/gooze/pkg"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

func TestLocalReportStore_SaveReports_WritesReportFields(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rs := &LocalReportStore{}

	report := m.Report{
		Source: rpt.Source{
			Origin: &rpt.File{FullPath: "/abs/path/file.go", Hash: "abc123"},
			Test:   &rpt.File{FullPath: "/abs/path/file_test.go", Hash: "def456"},
		},
		Result: m.Result{
			mgen.Boolean: {
				{MutationID: "m1", Status: rpt.Killed, Err: nil},
				{MutationID: "m2", Status: rpt.Error, Err: nil},
			},
			mgen.Arithmetic: {
				{MutationID: "m3", Status: rpt.Survived, Err: nil},
			},
		},
		Diff: nil,
	}

	expectedHash := rs.computeReportHash(report.Result)
	if expectedHash == "" {
		t.Fatalf("expected non-empty report hash")
	}

	if err := rs.SaveReports(context.Background(), m.Path(dir), reportSpill(t, report)); err != nil {
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

	var decoded rpt.Report
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
		if entry.Name != mgen.Boolean.Name {
			continue
		}
		for _, mut := range entry.Mutations {
			if mut.MutationID == "m2" {
				foundM2Err = true
				if mut.Err != "" {
					t.Fatalf("expected m2 err to be empty, got %q", mut.Err)
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
		Source: rpt.Source{Origin: &rpt.File{FullPath: "/abs/path/file.go", Hash: "abc123"}},
		Result: m.Result{},
		Diff:   nil,
	}

	if err := rs.SaveReports(context.Background(), m.Path(dir), reportSpill(t, report)); err != nil {
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
		Source: rpt.Source{Origin: &rpt.File{FullPath: "/abs/a.go", Hash: "sourceA"}},
		Result: m.Result{
			mgen.Boolean: {
				{MutationID: "b1", Status: rpt.Killed, Err: nil},
				{MutationID: "b2", Status: rpt.Skipped, Err: nil},
			},
		},
	}

	report2 := m.Report{
		Source: rpt.Source{Origin: &rpt.File{FullPath: "/abs/b.go", Hash: "sourceB"}},
		Result: m.Result{
			mgen.Arithmetic: {
				{MutationID: "a1", Status: rpt.Error, Err: nil},
			},
		},
	}

	if err := rs.SaveReports(context.Background(), m.Path(dir), reportSpill(t, report1, report2)); err != nil {
		t.Fatalf("SaveReports returned error: %v", err)
	}

	indexPath := filepath.Join(dir, "_index.yaml")
	if _, err := os.Stat(indexPath); err == nil {
		t.Fatalf("expected _index.yaml to not exist until RegenerateIndex is called")
	}

	if err := rs.RegenerateIndex(context.Background(), m.Path(dir)); err != nil {
		t.Fatalf("RegenerateIndex returned error: %v", err)
	}
	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("expected _index.yaml to exist: %v", err)
	}

	var idx rpt.Index
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

	bySource := map[string]rpt.IndexResult{}
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
	if reA.Mutations[0].MutationName != mgen.Boolean.Name {
		t.Fatalf("expected sourceA mutation_name=%q, got %q", mgen.Boolean.Name, reA.Mutations[0].MutationName)
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
	if reB.Mutations[0].MutationName != mgen.Arithmetic.Name {
		t.Fatalf("expected sourceB mutation_name=%q, got %q", mgen.Arithmetic.Name, reB.Mutations[0].MutationName)
	}
	if len(reB.Mutations[0].MutationReports) != 1 || reB.Mutations[0].MutationReports[0] != file2 {
		t.Fatalf("unexpected sourceB mutation_reports: %v", reB.Mutations[0].MutationReports)
	}
}

func TestLocalReportStore_SaveReports_WritesHashedYAMLPerReport(t *testing.T) {
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
		Source: rpt.Source{
			Origin: &rpt.File{FullPath: "/abs/path/file.go", Hash: "abc123"},
			Test:   &rpt.File{FullPath: "/abs/path/file_test.go", Hash: "def456"},
		},
		Result: m.Result{
			mgen.Boolean: {
				{MutationID: "m1", Status: rpt.Killed, Err: nil},
			},
		},
	}

	err = spill.Append(report)
	if err != nil {
		t.Fatalf("failed to append to filespill: %v", err)
	}

	expectedHash := rs.computeReportHash(report.Result)

	// Invoke method under test
	if err := rs.SaveReports(context.Background(), m.Path(dir), spill); err != nil {
		t.Fatalf("SaveReports returned error: %v", err)
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
		Source: rpt.Source{
			Origin: &rpt.File{FullPath: "/abs/path/file1.go", Hash: "abc123"},
			Test:   &rpt.File{FullPath: "/abs/path/file1_test.go", Hash: "def456"},
		},
		Result: m.Result{
			mgen.Boolean: {
				{MutationID: "m1", Status: rpt.Killed, Err: nil},
			},
		},
	}
	report2 := m.Report{
		Source: rpt.Source{
			Origin: &rpt.File{FullPath: "/abs/path/file2.go", Hash: "ghi789"},
			Test:   &rpt.File{FullPath: "/abs/path/file2_test.go", Hash: "jkl012"},
		},
		Result: m.Result{
			mgen.Arithmetic: {
				{MutationID: "m2", Status: rpt.Survived, Err: nil},
			},
		},
	}

	if err := rs.SaveReports(context.Background(), m.Path(dir), reportSpill(t, report1, report2)); err != nil {
		t.Fatalf("SaveReports returned error: %v", err)
	}

	// Now load them back into a spill
	loadedSpill, err := rs.LoadSpillReports(context.Background(), m.Path(dir))
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
		sources[r.Source.Origin.FullPath] = true
	}
	if !sources["/abs/path/file1.go"] || !sources["/abs/path/file2.go"] {
		t.Fatalf("loaded reports do not match expected sources")
	}
}

func TestLocalReportStore_CheckUpdates_NoReportsDir_ReturnsAllSources(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "does-not-exist")
	rs := &LocalReportStore{}

	sources := []rpt.Source{
		{Origin: &rpt.File{FullPath: "/abs/a.go", Hash: "hash-a"}},
		{Origin: &rpt.File{FullPath: "/abs/b.go", Hash: "hash-b"}},
	}

	changedSpill, deletedSpill, err := rs.CheckUpdates(context.Background(), m.Path(dir), sourceSpill(t, sources...), nil)
	changed := spillItems(t, changedSpill)
	_ = spillItems(t, deletedSpill)
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
	_, _, err := rs.CheckUpdates(context.Background(), "", sourceSpill(t), nil)
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
	_, _, err := rs.CheckUpdates(context.Background(), m.Path(filePath), sourceSpill(t), nil)
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

	sources := []rpt.Source{{Origin: &rpt.File{FullPath: "/abs/a.go", Hash: "hash-a"}}}
	changedSpill, deletedSpill, err := rs.CheckUpdates(context.Background(), m.Path(dir), sourceSpill(t, sources...), nil)
	changed := spillItems(t, changedSpill)
	_ = spillItems(t, deletedSpill)
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

	deleted := rpt.Source{Origin: &rpt.File{FullPath: "/abs/deleted.go", Hash: "old-hash"}}
	report := m.Report{
		Source: deleted,
		Result: m.Result{mgen.Boolean: {{MutationID: "m1", Status: rpt.Killed, Err: nil}}},
	}
	if err := rs.SaveReports(context.Background(), m.Path(dir), reportSpill(t, report)); err != nil {
		t.Fatalf("SaveReports returned error: %v", err)
	}

	changedSpill, deletedSpill, err := rs.CheckUpdates(context.Background(), m.Path(dir), sourceSpill(t), nil)
	if err != nil {
		t.Fatalf("CheckUpdates returned error: %v", err)
	}

	changed := spillItems(t, changedSpill)
	gone := spillItems(t, deletedSpill)

	if len(changed) != 0 {
		t.Fatalf("expected no changed sources, got %d", len(changed))
	}
	if len(gone) != 1 {
		t.Fatalf("expected 1 deleted source, got %d", len(gone))
	}
	if gone[0].Origin == nil || gone[0].Origin.FullPath != deleted.Origin.FullPath {
		t.Fatalf("expected deleted source %q, got %#v", deleted.Origin.FullPath, gone[0])
	}
}

func TestLocalReportStore_CheckUpdates_CodeOrTestChanged_ReturnsSource(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rs := &LocalReportStore{}

	old := rpt.Source{
		Origin: &rpt.File{FullPath: "/abs/a.go", Hash: "old-code"},
		Test:   &rpt.File{FullPath: "/abs/a_test.go", Hash: "old-test"},
	}
	report := m.Report{
		Source: old,
		Result: m.Result{mgen.Boolean: {{MutationID: "m1", Status: rpt.Killed, Err: nil}}},
	}
	if err := rs.SaveReports(context.Background(), m.Path(dir), reportSpill(t, report)); err != nil {
		t.Fatalf("SaveReports returned error: %v", err)
	}

	current := []rpt.Source{{
		Origin: &rpt.File{FullPath: "/abs/a.go", Hash: "new-code"},
		Test:   &rpt.File{FullPath: "/abs/a_test.go", Hash: "old-test"},
	}}

	changedSpill, deletedSpill, err := rs.CheckUpdates(context.Background(), m.Path(dir), sourceSpill(t, current...), nil)
	changed := spillItems(t, changedSpill)
	_ = spillItems(t, deletedSpill)
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
	stored := rpt.Source{
		Origin: &rpt.File{FullPath: "/abs/a.go", Hash: "same"},
		Test:   &rpt.File{FullPath: "/abs/a_test.go", Hash: "test-hash"},
	}
	report := m.Report{
		Source: stored,
		Result: m.Result{mgen.Boolean: {{MutationID: "m1", Status: rpt.Killed, Err: nil}}},
	}
	if err := rs.SaveReports(context.Background(), m.Path(dir), reportSpill(t, report)); err != nil {
		t.Fatalf("SaveReports returned error: %v", err)
	}

	// Current run has no test file associated.
	current := []rpt.Source{{Origin: &rpt.File{FullPath: "/abs/a.go", Hash: "same"}}}
	changedSpill, deletedSpill, err := rs.CheckUpdates(context.Background(), m.Path(dir), sourceSpill(t, current...), nil)
	changed := spillItems(t, changedSpill)
	_ = spillItems(t, deletedSpill)
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

	old := rpt.Source{Origin: &rpt.File{FullPath: "/abs/a.go", Hash: "same"}}

	// Previous run only had boolean mutations recorded.
	report := m.Report{
		Source: old,
		Result: m.Result{mgen.Boolean: {{MutationID: "m1", Status: rpt.Killed, Err: nil}}},
	}
	if err := rs.SaveReports(context.Background(), m.Path(dir), reportSpill(t, report)); err != nil {
		t.Fatalf("SaveReports returned error: %v", err)
	}

	current := []rpt.Source{{Origin: &rpt.File{FullPath: "/abs/a.go", Hash: "same"}}}
	changedSpill, deletedSpill, err := rs.CheckUpdates(context.Background(), m.Path(dir), sourceSpill(t, current...), nil)
	changed := spillItems(t, changedSpill)
	_ = spillItems(t, deletedSpill)
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

	old := rpt.Source{Origin: &rpt.File{FullPath: "/abs/a.go", Hash: "same"}}
	unknown := mgen.Type{Name: "custom-mut", Version: 1}
	report := m.Report{
		Source: old,
		Result: m.Result{unknown: {{MutationID: "m1", Status: rpt.Killed, Err: nil}}},
	}
	if err := rs.SaveReports(context.Background(), m.Path(dir), reportSpill(t, report)); err != nil {
		t.Fatalf("SaveReports returned error: %v", err)
	}

	current := []rpt.Source{{Origin: &rpt.File{FullPath: "/abs/a.go", Hash: "same"}}}
	changedSpill, deletedSpill, err := rs.CheckUpdates(context.Background(), m.Path(dir), sourceSpill(t, current...), nil)
	changed := spillItems(t, changedSpill)
	_ = spillItems(t, deletedSpill)
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

	sources := []rpt.Source{{Origin: nil}}
	changedSpill, deletedSpill, err := rs.CheckUpdates(context.Background(), m.Path(dir), sourceSpill(t, sources...), nil)
	changed := spillItems(t, changedSpill)
	_ = spillItems(t, deletedSpill)
	if err != nil {
		t.Fatalf("CheckUpdates returned error: %v", err)
	}
	if len(changed) != 1 {
		t.Fatalf("expected passthrough (no reports) to return all sources, got %d", len(changed))
	}
}

func TestLocalReportStore_CleanReports_DeletesOnlySelectedAndLeavesIndexToCaller(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rs := &LocalReportStore{}

	sourceA := rpt.Source{Origin: &rpt.File{FullPath: "/abs/a.go", Hash: "sourceA"}}
	sourceB := rpt.Source{Origin: &rpt.File{FullPath: "/abs/b.go", Hash: "sourceB"}}

	reportA := m.Report{Source: sourceA, Result: m.Result{mgen.Boolean: {{MutationID: "b1", Status: rpt.Killed, Err: nil}}}}
	reportB := m.Report{Source: sourceB, Result: m.Result{mgen.Arithmetic: {{MutationID: "a1", Status: rpt.Survived, Err: nil}}}}

	if err := rs.SaveReports(context.Background(), m.Path(dir), reportSpill(t, reportA, reportB)); err != nil {
		t.Fatalf("SaveReports returned error: %v", err)
	}
	if err := rs.RegenerateIndex(context.Background(), m.Path(dir)); err != nil {
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

	if err := rs.CleanReports(context.Background(), m.Path(dir), sourceSpill(t, sourceA)); err != nil {
		t.Fatalf("CleanReports returned error: %v", err)
	}

	if _, err := os.Stat(fileA); err == nil {
		t.Fatalf("expected report A file to be deleted")
	}
	if _, err := os.Stat(fileB); err != nil {
		t.Fatalf("expected report B file to remain: %v", err)
	}

	stale, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read _index.yaml: %v", err)
	}
	var staleIdx rpt.Index
	if err := yaml.Unmarshal(stale, &staleIdx); err != nil {
		t.Fatalf("unmarshal _index.yaml: %v", err)
	}
	if len(staleIdx.Result) != 2 {
		t.Fatalf("CleanReports must not rewrite the index; expected 2 entries, got %d", len(staleIdx.Result))
	}

	if err := rs.RegenerateIndex(context.Background(), m.Path(dir)); err != nil {
		t.Fatalf("RegenerateIndex returned error: %v", err)
	}

	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read _index.yaml: %v", err)
	}
	var idx rpt.Index
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

	sourceA := rpt.Source{Origin: &rpt.File{FullPath: "/abs/a.go", Hash: "sourceA"}}
	reportA := m.Report{Source: sourceA, Result: m.Result{mgen.Boolean: {{MutationID: "b1", Status: rpt.Killed, Err: nil}}}}

	if err := rs.SaveReports(context.Background(), m.Path(dir), reportSpill(t, reportA)); err != nil {
		t.Fatalf("SaveReports returned error: %v", err)
	}
	if err := rs.RegenerateIndex(context.Background(), m.Path(dir)); err != nil {
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

	if err := rs.CleanReports(context.Background(), m.Path(dir), sourceSpill(t, sourceA)); err != nil {
		t.Fatalf("CleanReports returned error: %v", err)
	}

	if _, err := os.Stat(fileA); err == nil {
		t.Fatalf("expected report file to be deleted")
	}
	if err := rs.RegenerateIndex(context.Background(), m.Path(dir)); err != nil {
		t.Fatalf("RegenerateIndex returned error: %v", err)
	}
	if _, err := os.Stat(indexPath); err == nil {
		t.Fatalf("expected _index.yaml to be deleted")
	}
}

func TestLocalReportStore_CleanReports_NoReportsDir_NoError(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "does-not-exist")
	rs := &LocalReportStore{}

	if err := rs.CleanReports(context.Background(), m.Path(dir), sourceSpill(t)); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestLocalReportStore_CleanReports_EmptyPath_ReturnsError(t *testing.T) {
	t.Parallel()

	rs := &LocalReportStore{}
	if err := rs.CleanReports(context.Background(), "", sourceSpill(t)); err == nil {
		t.Fatalf("expected error")
	}
}

func TestLocalReportStore_CheckUpdates_MutatorVersionDiff_ReturnsSource(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rs := &LocalReportStore{}

	old := rpt.Source{Origin: &rpt.File{FullPath: "/abs/a.go", Hash: "same"}}

	// Simulate a stored report with an older boolean mutator version.
	oldBool := mgen.Type{Name: mgen.Boolean.Name, Version: 0}
	report := m.Report{
		Source: old,
		Result: m.Result{oldBool: {{MutationID: "m1", Status: rpt.Killed, Err: nil}}},
	}
	if err := rs.SaveReports(context.Background(), m.Path(dir), reportSpill(t, report)); err != nil {
		t.Fatalf("SaveReports returned error: %v", err)
	}

	current := []rpt.Source{{Origin: &rpt.File{FullPath: "/abs/a.go", Hash: "same"}}}
	changedSpill, deletedSpill, err := rs.CheckUpdates(context.Background(), m.Path(dir), sourceSpill(t, current...), nil)
	changed := spillItems(t, changedSpill)
	_ = spillItems(t, deletedSpill)
	if err != nil {
		t.Fatalf("CheckUpdates returned error: %v", err)
	}
	if len(changed) != 1 {
		t.Fatalf("expected 1 changed source due to mutator version diff, got %d", len(changed))
	}
}

func TestLocalReportStore_CheckUpdates_ComparisonV1ReportsAreRetested(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rs := &LocalReportStore{}

	// Reports written before comparison v2 still hold mutants it no longer generates.
	source := rpt.Source{Origin: &rpt.File{FullPath: "/abs/a.go", Hash: "same"}}
	comparisonV1 := mgen.Type{Name: mgen.Comparison.Name, Version: 1}
	report := m.Report{Source: source, Result: m.Result{comparisonV1: {{MutationID: "m1", Status: rpt.Survived}}}}

	if err := rs.SaveReports(context.Background(), m.Path(dir), reportSpill(t, report)); err != nil {
		t.Fatalf("SaveReports returned error: %v", err)
	}

	changedSpill, deletedSpill, err := rs.CheckUpdates(context.Background(), m.Path(dir), sourceSpill(t, source), nil)
	if err != nil {
		t.Fatalf("CheckUpdates returned error: %v", err)
	}

	_ = spillItems(t, deletedSpill)
	if changed := spillItems(t, changedSpill); len(changed) != 1 {
		t.Fatalf("expected the comparison v1 source to be re-tested, got %d changed sources", len(changed))
	}
}

func TestLocalReportStore_RegenerateIndex_ListsReportsPerSource(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rs := &LocalReportStore{}

	reports := []m.Report{
		{
			Source: rpt.Source{Origin: &rpt.File{FullPath: "/abs/a.go", Hash: "sourceA"}},
			Result: m.Result{mgen.Boolean: {{MutationID: "a1", Status: rpt.Killed}}},
		},
		{
			Source: rpt.Source{Origin: &rpt.File{FullPath: "/abs/b.go", Hash: "sourceB"}},
			Result: m.Result{mgen.Boolean: {{MutationID: "b1", Status: rpt.Survived}}},
		},
	}

	require.NoError(t, rs.SaveReports(context.Background(), m.Path(dir), reportSpill(t, reports...)))
	require.NoError(t, rs.RegenerateIndex(context.Background(), m.Path(dir)))

	data, err := os.ReadFile(filepath.Join(dir, "_index.yaml"))
	require.NoError(t, err)

	var idx rpt.Index
	require.NoError(t, yaml.Unmarshal(data, &idx))
	require.Len(t, idx.Result, 2)

	for i, report := range reports {
		entry := idx.Result[i]
		if entry.SourceHex != report.Source.Origin.Hash {
			entry = idx.Result[1-i]
		}

		require.Len(t, entry.Mutations, 1)
		require.Equal(t, []string{rs.computeReportHash(report.Result) + ".yaml"}, entry.Mutations[0].MutationReports)
	}
}

func sourceSpill(t *testing.T, sources ...rpt.Source) pkg.FileSpill[rpt.Source] {
	t.Helper()

	spill, err := pkg.FromSlice(sources)
	require.NoError(t, err)
	t.Cleanup(func() { _ = spill.Close() })

	return spill
}

func spillItems[T any](t *testing.T, spill pkg.FileSpill[T]) []T {
	t.Helper()

	require.NotNil(t, spill)
	t.Cleanup(func() { _ = spill.Close() })

	items, err := pkg.ToSlice(spill)
	require.NoError(t, err)

	return items
}

func TestLocalReportStore_CheckUpdates_ChangedKeepsScanOrderAndDeletedIsSorted(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rs := &LocalReportStore{}

	stored := []m.Report{
		{Source: rpt.Source{Origin: &rpt.File{ShortPath: "z.go", FullPath: "/old/z.go", Hash: "z"}}, Result: m.Result{mgen.Boolean: {{MutationID: "z1", Status: rpt.Killed}}}},
		{Source: rpt.Source{Origin: &rpt.File{ShortPath: "a.go", FullPath: "/old/a.go", Hash: "a"}}, Result: m.Result{mgen.Boolean: {{MutationID: "a1", Status: rpt.Killed}}}},
		{Source: rpt.Source{Origin: &rpt.File{ShortPath: "keep.go", FullPath: "/old/keep.go", Hash: "k"}}, Result: m.Result{mgen.Boolean: {{MutationID: "k1", Status: rpt.Killed}}}},
	}
	require.NoError(t, rs.SaveReports(context.Background(), m.Path(dir), reportSpill(t, stored...)))

	current := []rpt.Source{
		{Origin: &rpt.File{ShortPath: "new2.go", FullPath: "/new/new2.go", Hash: "n2"}},
		{Origin: &rpt.File{ShortPath: "keep.go", FullPath: "/new/keep.go", Hash: "k"}},
		{Origin: &rpt.File{ShortPath: "new1.go", FullPath: "/new/new1.go", Hash: "n1"}},
	}

	changedSpill, deletedSpill, err := rs.CheckUpdates(context.Background(), m.Path(dir), sourceSpill(t, current...), nil)
	require.NoError(t, err)

	changed := spillItems(t, changedSpill)
	deleted := spillItems(t, deletedSpill)

	require.Equal(t, []string{"new2.go", "new1.go"}, shortPaths(changed))
	require.Equal(t, []string{"a.go", "z.go"}, shortPaths(deleted))
}

func shortPaths(sources []rpt.Source) []string {
	paths := make([]string, 0, len(sources))
	for _, source := range sources {
		paths = append(paths, source.Origin.ShortPath)
	}

	return paths
}

func TestLocalReportStore_LoadSpillReports_FailsOnCorruptReport(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rs := &LocalReportStore{}

	require.NoError(t, os.WriteFile(filepath.Join(dir, "broken.yaml"), []byte("source: [\n"), 0o600))

	_, err := rs.LoadSpillReports(context.Background(), m.Path(dir))
	require.ErrorContains(t, err, "broken.yaml")
}

func TestLocalReportStore_RegenerateIndex_ListsEachReportFileOnce(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rs := &LocalReportStore{}

	source := rpt.Source{Origin: &rpt.File{FullPath: "/abs/a.go", Hash: "sourceA"}}
	reports := []m.Report{
		{Source: source, Result: m.Result{mgen.Boolean: {{MutationID: "b1", Status: rpt.Killed}}}},
		{Source: source, Result: m.Result{mgen.Boolean: {{MutationID: "b2", Status: rpt.Survived}}}},
	}
	require.NoError(t, rs.SaveReports(context.Background(), m.Path(dir), reportSpill(t, reports...)))
	require.NoError(t, rs.RegenerateIndex(context.Background(), m.Path(dir)))

	data, err := os.ReadFile(filepath.Join(dir, "_index.yaml"))
	require.NoError(t, err)

	var idx rpt.Index
	require.NoError(t, yaml.Unmarshal(data, &idx))
	require.Equal(t, 2, idx.TotalMutations)
	require.Len(t, idx.Result, 1)
	require.Len(t, idx.Result[0].Mutations, 1)
	require.Len(t, idx.Result[0].Mutations[0].MutationReports, 2)
}

func reportSpill(t *testing.T, reports ...m.Report) pkg.FileSpill[m.Report] {
	t.Helper()

	spill, err := pkg.FromSlice(reports)
	require.NoError(t, err)
	t.Cleanup(func() { _ = spill.Close() })

	return spill
}

func TestLocalReportStore_MarshalReport_WritesErrorString(t *testing.T) {
	t.Parallel()

	rs := &LocalReportStore{}
	report := m.Report{
		Source: rpt.Source{Origin: &rpt.File{FullPath: "/abs/a.go", Hash: "h"}},
		Result: m.Result{mgen.Boolean: {{MutationID: "m2", Status: rpt.Error, Err: errors.New("boom")}}},
	}

	data, err := rs.marshalReport(report)
	require.NoError(t, err)
	require.Contains(t, string(data), "boom")
}

func closedSourceSpill(t *testing.T, sources ...rpt.Source) pkg.FileSpill[rpt.Source] {
	t.Helper()

	spill, err := pkg.FromSlice(sources)
	require.NoError(t, err)
	require.NoError(t, spill.Close())

	return spill
}

func TestLocalReportStore_RangeErrors(t *testing.T) {
	rs := &LocalReportStore{}
	ctx := context.Background()

	corrupt := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(corrupt, "bad.yaml"), []byte("source: [\n"), 0o600))

	unreadable := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(unreadable, "dir.yaml"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(unreadable, "locked.yaml"), []byte("result: []\n"), 0o000))

	t.Run("CheckUpdates on corrupt reports", func(t *testing.T) {
		_, _, err := rs.CheckUpdates(ctx, m.Path(corrupt), sourceSpill(t), nil)
		require.ErrorContains(t, err, "bad.yaml")
	})

	t.Run("RegenerateIndex on corrupt reports", func(t *testing.T) {
		require.ErrorContains(t, rs.RegenerateIndex(ctx, m.Path(corrupt)), "bad.yaml")
	})

	t.Run("unreadable report file", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root can read files without permission bits")
		}

		_, err := rs.LoadSpillReports(ctx, m.Path(unreadable))
		require.ErrorContains(t, err, "locked.yaml")
	})

	t.Run("CleanReports with closed sources spill", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, rs.SaveReports(ctx, m.Path(dir), reportSpill(t, m.Report{
			Source: rpt.Source{Origin: &rpt.File{FullPath: "/abs/a.go", Hash: "h"}},
			Result: m.Result{mgen.Boolean: {{MutationID: "b1", Status: rpt.Killed}}},
		})))

		require.ErrorContains(t, rs.CleanReports(ctx, m.Path(dir), closedSourceSpill(t, rpt.Source{Origin: &rpt.File{FullPath: "/abs/a.go"}})), "read sources to clean")
	})

	t.Run("CleanReports with corrupt report", func(t *testing.T) {
		err := rs.CleanReports(ctx, m.Path(corrupt), sourceSpill(t, rpt.Source{Origin: &rpt.File{FullPath: "/abs/a.go", Hash: "h"}}))
		require.ErrorContains(t, err, "bad.yaml")
	})

	t.Run("CheckUpdates with closed sources spill", func(t *testing.T) {
		_, _, err := rs.CheckUpdates(ctx, m.Path(t.TempDir()), closedSourceSpill(t, rpt.Source{Origin: &rpt.File{FullPath: "/abs/a.go"}}), nil)
		require.ErrorContains(t, err, "split changed sources")
	})

	t.Run("spill creation fails", func(t *testing.T) {
		sources := sourceSpill(t)
		t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))

		_, _, err := rs.CheckUpdates(ctx, m.Path(t.TempDir()), sources, nil)
		require.ErrorContains(t, err, "create changed sources spill")

		_, err = rs.LoadSpillReports(ctx, m.Path(t.TempDir()))
		require.ErrorContains(t, err, "create filespill")
	})
}

func TestLocalReportStore_CleanReports_IgnoresNilAndOriginlessSources(t *testing.T) {
	rs := &LocalReportStore{}
	ctx := context.Background()
	dir := t.TempDir()

	report := m.Report{
		Source: rpt.Source{Origin: &rpt.File{FullPath: "/abs/a.go", Hash: "h"}},
		Result: m.Result{mgen.Boolean: {{MutationID: "b1", Status: rpt.Killed}}},
	}
	require.NoError(t, rs.SaveReports(ctx, m.Path(dir), reportSpill(t, report)))

	require.NoError(t, rs.CleanReports(ctx, m.Path(dir), nil))
	require.NoError(t, rs.CleanReports(ctx, m.Path(dir), sourceSpill(t, rpt.Source{})))

	reports := spillItems(t, mustLoad(t, rs, dir))
	require.Len(t, reports, 1)
}

func mustLoad(t *testing.T, rs *LocalReportStore, dir string) pkg.FileSpill[m.Report] {
	t.Helper()

	spill, err := rs.LoadSpillReports(context.Background(), m.Path(dir))
	require.NoError(t, err)

	return spill
}

func TestLocalReportStore_CheckUpdates_InconsistentStoredVersionsReturnSource(t *testing.T) {
	rs := &LocalReportStore{}
	ctx := context.Background()
	dir := t.TempDir()

	source := rpt.Source{Origin: &rpt.File{FullPath: "/abs/a.go", Hash: "same"}}
	reports := []m.Report{
		{Source: source, Result: m.Result{mgen.Boolean: {{MutationID: "b1", Status: rpt.Killed}}}},
		{Source: source, Result: m.Result{mgen.Type{Name: mgen.Boolean.Name, Version: mgen.Boolean.Version + 1}: {{MutationID: "b2", Status: rpt.Killed}}}},
		{Source: rpt.Source{}, Result: m.Result{mgen.Boolean: {{MutationID: "orphan", Status: rpt.Killed}}}},
	}
	require.NoError(t, rs.SaveReports(ctx, m.Path(dir), reportSpill(t, reports...)))

	changed, deleted, err := rs.CheckUpdates(ctx, m.Path(dir), sourceSpill(t, source), nil)
	require.NoError(t, err)
	require.Len(t, spillItems(t, changed), 1)
	require.Empty(t, spillItems(t, deleted))
}

func TestLocalReportStore_RegenerateIndex_SkipsSourcesWithoutHash(t *testing.T) {
	rs := &LocalReportStore{}
	ctx := context.Background()
	dir := t.TempDir()

	require.NoError(t, rs.SaveReports(ctx, m.Path(dir), reportSpill(t, m.Report{
		Source: rpt.Source{Origin: &rpt.File{FullPath: "/abs/a.go"}},
		Result: m.Result{mgen.Boolean: {{MutationID: "b1", Status: rpt.Killed}}},
	})))
	require.NoError(t, rs.RegenerateIndex(ctx, m.Path(dir)))

	data, err := os.ReadFile(filepath.Join(dir, "_index.yaml"))
	require.NoError(t, err)

	var idx rpt.Index
	require.NoError(t, yaml.Unmarshal(data, &idx))
	require.Equal(t, 1, idx.TotalMutations)
	require.Empty(t, idx.Result)
}

func TestLocalReportStore_AddStoredReport_MarksAnyVersionMismatch(t *testing.T) {
	rs := &LocalReportStore{}
	source := rpt.Source{Origin: &rpt.File{ShortPath: "a.go", FullPath: "/abs/a.go", Hash: "h"}}
	older := mgen.Type{Name: mgen.Boolean.Name, Version: mgen.Boolean.Version - 1}

	state := make(map[string]storedSourceState)
	rs.addStoredReport(state, m.Report{Source: source, Result: m.Result{mgen.Boolean: {{MutationID: "b1", Status: rpt.Killed}}}})
	rs.addStoredReport(state, m.Report{Source: source, Result: m.Result{older: {{MutationID: "b2", Status: rpt.Killed}}}})

	require.Equal(t, -1, state["a.go"].mutator[mgen.Boolean.Name])
	require.True(t, rs.isSourceChanged(state, "a.go", source))
}

func TestLocalReportStore_AddStoredReport_MarksMismatchInEitherOrder(t *testing.T) {
	rs := &LocalReportStore{}
	source := rpt.Source{Origin: &rpt.File{ShortPath: "a.go", FullPath: "/abs/a.go", Hash: "h"}}
	older := mgen.Type{Name: mgen.Boolean.Name, Version: mgen.Boolean.Version - 1}

	state := make(map[string]storedSourceState)
	rs.addStoredReport(state, m.Report{Source: source, Result: m.Result{older: {{MutationID: "b2", Status: rpt.Killed}}}})
	rs.addStoredReport(state, m.Report{Source: source, Result: m.Result{mgen.Boolean: {{MutationID: "b1", Status: rpt.Killed}}}})

	require.Equal(t, -1, state["a.go"].mutator[mgen.Boolean.Name])
}

func TestLocalReportStore_LoadSpillReports_ReturnsErrorWhenReportUnreadable(t *testing.T) {
	rs := &LocalReportStore{}
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte("source: [\n"), 0o600))

	_, err := rs.LoadSpillReports(context.Background(), m.Path(dir))
	require.ErrorContains(t, err, "load reports into spill")
}

func TestLocalReportStore_RegenerateIndex_OmitsReportsWithoutResults(t *testing.T) {
	rs := &LocalReportStore{}
	builder := newIndexBuilder(rs)

	builder.add(m.Report{Source: rpt.Source{Origin: &rpt.File{FullPath: "/abs/empty.go", Hash: "empty"}}, Result: m.Result{}})
	builder.add(m.Report{
		Source: rpt.Source{Origin: &rpt.File{FullPath: "/abs/a.go", Hash: "a"}},
		Result: m.Result{mgen.Boolean: {{MutationID: "b1", Status: rpt.Killed}}},
	})

	index := builder.build()
	require.Len(t, index.Result, 1)
	require.Equal(t, "a", index.Result[0].SourceHex)
}

func goldenReports() []m.Report {
	pkgName := "calc"
	diff := []byte("--- original\n+++ mutated\n@@ -1 +1 @@\n-a + b\n+a - b\n")

	return []m.Report{
		{
			Source: rpt.Source{
				Origin:  &rpt.File{ShortPath: "calc/add.go", FullPath: "/repo/calc/add.go", Hash: "aaaa"},
				Test:    &rpt.File{ShortPath: "calc/add_test.go", FullPath: "/repo/calc/add_test.go", Hash: "bbbb"},
				Package: &pkgName,
			},
			Result: m.Result{
				mgen.Arithmetic: {
					{MutationID: "m1", Status: rpt.Killed},
					{MutationID: "m2", Status: rpt.Survived},
				},
				mgen.Boolean: {
					{MutationID: "m3", Status: rpt.Error},
				},
			},
			Diff: &diff,
		},
		{
			Source: rpt.Source{Origin: &rpt.File{ShortPath: "calc/sub.go", FullPath: "/repo/calc/sub.go", Hash: "cccc"}},
			Result: m.Result{
				mgen.Numbers: {
					{MutationID: "m4", Status: rpt.Timeout},
					{MutationID: "m5", Status: rpt.Skipped},
					{MutationID: "m6", Status: rpt.NotCovered},
				},
			},
		},
	}
}

func TestLocalReportStore_SaveReports_MatchesGoldenFormat(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rs := &LocalReportStore{}

	require.NoError(t, rs.SaveReports(context.Background(), m.Path(dir), reportSpill(t, goldenReports()...)))
	require.NoError(t, rs.RegenerateIndex(context.Background(), m.Path(dir)))

	golden := filepath.Join("..", "..", "testdata", "report")

	want, err := os.ReadDir(golden)
	require.NoError(t, err)

	got, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, got, len(want))

	for i, entry := range want {
		require.Equal(t, entry.Name(), got[i].Name())

		wantData, err := os.ReadFile(filepath.Join(golden, entry.Name()))
		require.NoError(t, err)

		gotData, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		require.NoError(t, err)
		require.Equal(t, string(wantData), string(gotData), entry.Name())
	}
}

func TestLocalReportStore_CheckUpdates_EveryRegisteredMutatorIsKnown(t *testing.T) {
	t.Parallel()

	for _, mg := range mgen.All() {
		dir := t.TempDir()
		rs := &LocalReportStore{}
		source := rpt.Source{Origin: &rpt.File{FullPath: "/abs/a.go", Hash: "same"}}
		report := m.Report{Source: source, Result: m.Result{mg.Type: {{MutationID: "m1", Status: rpt.Killed}}}}

		require.NoError(t, rs.SaveReports(context.Background(), m.Path(dir), reportSpill(t, report)))

		changedSpill, deletedSpill, err := rs.CheckUpdates(context.Background(), m.Path(dir), sourceSpill(t, source), nil)
		require.NoError(t, err)
		require.Empty(t, spillItems(t, changedSpill), mg.Name)
		require.Empty(t, spillItems(t, deletedSpill), mg.Name)
	}
}

func TestLocalReportStore_CleanReports_ReturnsRemoveError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can remove files from read-only directories")
	}

	rs := &LocalReportStore{}
	ctx := context.Background()
	dir := t.TempDir()
	source := rpt.Source{Origin: &rpt.File{FullPath: "/abs/a.go", Hash: "h"}}

	require.NoError(t, rs.SaveReports(ctx, m.Path(dir), reportSpill(t, m.Report{
		Source: source,
		Result: m.Result{mgen.Boolean: {{MutationID: "b1", Status: rpt.Killed}}},
	})))
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	require.ErrorContains(t, rs.CleanReports(ctx, m.Path(dir), sourceSpill(t, source)), "remove report file")
}

func TestLocalReportStore_CheckUpdates_SourcesOutsideTheScanAreNotDeleted(t *testing.T) {
	rs := &LocalReportStore{}
	ctx := context.Background()
	project := t.TempDir()
	t.Chdir(project)
	require.NoError(t, os.WriteFile(filepath.Join(project, "kept.go"), []byte("package p\n"), 0o600))

	dir := filepath.Join(project, "reports")
	kept := rpt.Source{Origin: &rpt.File{ShortPath: "kept.go", FullPath: "/elsewhere/kept.go", Hash: "k"}}
	gone := rpt.Source{Origin: &rpt.File{ShortPath: "gone.go", FullPath: "/elsewhere/gone.go", Hash: "g"}}

	require.NoError(t, rs.SaveReports(ctx, m.Path(dir), reportSpill(t,
		m.Report{Source: kept, Result: m.Result{mgen.Boolean: {{MutationID: "k1", Status: rpt.Killed}}}},
		m.Report{Source: gone, Result: m.Result{mgen.Boolean: {{MutationID: "g1", Status: rpt.Killed}}}},
	)))

	changed, deleted, err := rs.CheckUpdates(ctx, m.Path(dir), sourceSpill(t), nil)
	require.NoError(t, err)
	require.Empty(t, spillItems(t, changed))
	require.Equal(t, []string{"gone.go"}, shortPaths(spillItems(t, deleted)))
}

func TestLocalReportStore_SaveAndLoad_KeepsLineAndScope(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rs := &LocalReportStore{}

	scope := &rpt.Scope{Base: "1a2b3c4", Lines: []rpt.LineRange{{Start: 3, End: 4}}}
	report := m.Report{
		Source: rpt.Source{Origin: &rpt.File{FullPath: "/abs/a.go", Hash: "h"}},
		Result: m.Result{mgen.Boolean: {{MutationID: "m1", Status: rpt.Survived}}},
		Line:   3,
		Scope:  scope,
	}
	require.NoError(t, rs.SaveReports(context.Background(), m.Path(dir), reportSpill(t, report)))

	loaded, err := rs.LoadSpillReports(context.Background(), m.Path(dir))
	require.NoError(t, err)

	got := spillItems(t, loaded)
	require.Len(t, got, 1)
	assert.Equal(t, 3, got[0].Line)
	assert.Equal(t, scope, got[0].Scope)
}

func TestLocalReportStore_CheckUpdates_PartlyTestedSourceIsChanged(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rs := &LocalReportStore{}

	partial := rpt.Source{Origin: &rpt.File{FullPath: "/abs/partial.go", Hash: "same-p"}}
	full := rpt.Source{Origin: &rpt.File{FullPath: "/abs/full.go", Hash: "same-f"}}
	result := m.Result{mgen.Boolean: {{MutationID: "m1", Status: rpt.Killed}}}

	reports := []m.Report{
		// A diff-mode run tested only line 3 of partial.go...
		{Source: partial, Result: result, Line: 3, Scope: &rpt.Scope{Base: "abc", Lines: []rpt.LineRange{{Start: 3, End: 3}}}},
		{Source: partial, Result: m.Result{mgen.Arithmetic: {{MutationID: "m2", Status: rpt.Killed}}}, Line: 3},
		// ...while full.go was tested in full.
		{Source: full, Result: m.Result{mgen.Boolean: {{MutationID: "m3", Status: rpt.Killed}}}, Line: 1},
	}
	require.NoError(t, rs.SaveReports(context.Background(), m.Path(dir), reportSpill(t, reports...)))

	changedSpill, deletedSpill, err := rs.CheckUpdates(context.Background(), m.Path(dir), sourceSpill(t, partial, full), nil)
	require.NoError(t, err)

	changed := spillItems(t, changedSpill)
	_ = spillItems(t, deletedSpill)

	// Unchanged hashes don't make partial.go cached: its other lines were
	// never tested, so the next run re-tests it.
	assert.Equal(t, []rpt.Source{partial}, changed)
}

func TestLocalReportStore_CheckUpdates_AsksUntestedAboutCurrentSources(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rs := &LocalReportStore{}
	ctx := context.Background()

	current := rpt.Source{Origin: &rpt.File{FullPath: "/abs/a.go", Hash: "same"}}
	untested := rpt.Source{Origin: &rpt.File{FullPath: "/abs/b.go", Hash: "same"}}
	edited := rpt.Source{Origin: &rpt.File{FullPath: "/abs/c.go", Hash: "old"}}
	added := rpt.Source{Origin: &rpt.File{FullPath: "/abs/d.go", Hash: "new"}}

	require.NoError(t, rs.SaveReports(ctx, m.Path(dir), reportSpill(t,
		m.Report{Source: current, Result: m.Result{mgen.Unary: {{MutationID: "a1", Status: rpt.Killed}}}},
		m.Report{Source: current, Result: m.Result{mgen.Arithmetic: {{MutationID: "a2", Status: rpt.Killed}}}},
		m.Report{Source: untested, Result: m.Result{mgen.Boolean: {{MutationID: "b1", Status: rpt.Killed}}}},
		m.Report{Source: edited, Result: m.Result{mgen.Boolean: {{MutationID: "c1", Status: rpt.Killed}}}},
	)))

	asked := map[string][]string{}
	editedNow := rpt.Source{Origin: &rpt.File{FullPath: "/abs/c.go", Hash: "new"}}

	changedSpill, deletedSpill, err := rs.CheckUpdates(ctx, m.Path(dir), sourceSpill(t, current, untested, editedNow, added),
		func(source rpt.Source, tested []string) bool {
			asked[source.Key()] = tested
			return source.Key() == "/abs/b.go"
		})
	require.NoError(t, err)
	_ = spillItems(t, deletedSpill)

	assert.Equal(t, []rpt.Source{untested, editedNow, added}, spillItems(t, changedSpill),
		"a source untested says yes to is re-tested, like an edited or a new one")
	assert.Equal(t, map[string][]string{
		"/abs/a.go": {"arithmetic", "unary"},
		"/abs/b.go": {"boolean"},
	}, asked, "only sources whose reports are otherwise current are asked about, with their mutagens sorted")
}
