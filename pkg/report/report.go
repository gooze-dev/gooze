// Package report defines the files gooze writes to its reports directory: one
// YAML file per tested mutation batch and an `_index.yaml` summary. It reads
// them back, locates the changes in their diffs and computes the mutation score
// the way gooze does.
package report

import "cmp"

// IndexFileName is the name of the summary file in a reports directory.
const IndexFileName = "_index.yaml"

// SelectionFileName is the file in which a narrowed run records its Selection
// next to its reports. It is JSON, so readers that take every YAML file in
// the directory for a report skip it.
const SelectionFileName = "_selection.json"

// Selection records what a narrowed run (--diff, --run or --mutagens) chose to
// test, so a later report view can tell a diff run's reports from others.
type Selection struct {
	// Diff is the git ref a --diff run took its changes against, and Base the
	// merge-base commit it resolved to.
	Diff string `json:"diff,omitempty"`
	Base string `json:"base,omitempty"`
	// Run is the --run pattern.
	Run string `json:"run,omitempty"`
	// Mutagens are the --mutagens names.
	Mutagens []string `json:"mutagens,omitempty"`
}

// TestStatus is the outcome of testing one mutation. It is stored in report
// files as its integer value.
type TestStatus int

const (
	// Killed indicates the mutation was detected by tests.
	Killed TestStatus = iota
	// Survived indicates the mutation was not detected by tests.
	Survived
	// Skipped indicates the mutation was skipped.
	Skipped
	// Timeout indicates the mutation test timed out.
	Timeout
	// Error indicates an error occurred during testing, typically a mutation
	// that does not compile.
	Error
	// NotCovered indicates the mutated line is not exercised by any test, so the
	// mutation survives without running tests.
	NotCovered
)

// String returns the lower-case name of the status.
func (t TestStatus) String() string {
	switch t {
	case Killed:
		return "killed"
	case Survived:
		return "survived"
	case Skipped:
		return "skipped"
	case Error:
		return "error"
	case Timeout:
		return "timeout"
	case NotCovered:
		return "not_covered"
	default:
		return "unknown"
	}
}

// File identifies a file on disk and the hash of its content.
type File struct {
	ShortPath string `yaml:"shortpath"`
	FullPath  string `yaml:"fullpath"`
	Hash      string `yaml:"hash"`
}

// Source is a mutated Go file, its test file and its package name. Test and
// Package are nil when unknown.
type Source struct {
	Origin  *File   `yaml:"origin"`
	Test    *File   `yaml:"test"`
	Package *string `yaml:"package"`
}

// Key identifies the origin file across machines: the project-relative
// ShortPath when known, otherwise FullPath. Empty when there is no origin.
func (s Source) Key() string {
	if s.Origin == nil {
		return ""
	}

	return cmp.Or(s.Origin.ShortPath, s.Origin.FullPath)
}

// Report is the content of one report file: the results of the mutations gooze
// tested together for a source, and the unified diff of a surviving mutation.
type Report struct {
	Source Source   `yaml:"source"`
	Result []Result `yaml:"result"`
	Diff   *[]byte  `yaml:"diff"`
	// Line is the 1-based source line of the mutation, or 0 in reports
	// written before gooze recorded it (the diff still locates it).
	Line int `yaml:"line,omitempty"`
	// Scope is set when the run tested only part of the source (some lines,
	// some functions or some mutagens); nil means it was tested in full.
	Scope *Scope `yaml:"scope,omitempty"`
}

// Scope records how a run narrowed what it tested of a source.
type Scope struct {
	// Base is the commit a diff-mode run took the changes against.
	Base string `yaml:"base,omitempty"`
	// Lines are the line ranges a diff-mode run tested.
	Lines []LineRange `yaml:"lines,omitempty"`
	// Run is the --run pattern that selected the functions tested.
	Run string `yaml:"run,omitempty"`
	// Mutagens are the only mutagens the run used.
	Mutagens []string `yaml:"mutagens,omitempty"`
	// Interrupted is set when the run was stopped before it tested every
	// mutation of the source; the next run tests the rest.
	Interrupted bool `yaml:"interrupted,omitempty"`
}

// LineRange is an inclusive range of 1-based line numbers.
type LineRange struct {
	Start int `yaml:"start"`
	End   int `yaml:"end"`
}

// Result groups the mutation results of one mutagen and its version.
type Result struct {
	Name      string           `yaml:"name"`
	Version   int              `yaml:"version"`
	Mutations []MutationResult `yaml:"mutations"`
}

// MutationResult is the outcome of one mutation. Err holds the error message,
// if any.
type MutationResult struct {
	MutationID string     `yaml:"mutationid"`
	Status     TestStatus `yaml:"status"`
	Err        string     `yaml:"err,omitempty"`
}

// Index is the content of IndexFileName: totals per status and, per source
// hash and mutagen, the report files that hold its results. FailedMutations
// counts Error and Timeout; IgnoredMutations counts Skipped.
type Index struct {
	TotalMutations      int           `yaml:"total_mutations"`
	KilledMutations     int           `yaml:"killed_mutations"`
	SurvivedMutations   int           `yaml:"survived_mutations"`
	FailedMutations     int           `yaml:"failed_mutations"`
	IgnoredMutations    int           `yaml:"ignored_mutations"`
	NotCoveredMutations int           `yaml:"not_covered_mutations"`
	Result              []IndexResult `yaml:"result"`
}

// IndexResult lists the report files of one source, identified by its hash.
type IndexResult struct {
	SourceHex string          `yaml:"source_hex"`
	Mutations []IndexMutation `yaml:"mutations"`
}

// IndexMutation lists the report files holding results of one mutagen.
type IndexMutation struct {
	MutationName    string   `yaml:"mutation_name"`
	MutationReports []string `yaml:"mutation_reports"`
}
