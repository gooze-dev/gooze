package model

// Report represents the result of testing a mutation.
type Report struct {
	MutationID string
	SourceFile Path   // source file that was mutated
	Killed     bool   // true if test detected the mutation (test failed)
	Output     string // test output/error message
	Error      error  // error executing test (not test failure)
}

// FileResult holds the mutation testing results for a single source file.
type FileResult struct {
	Source  Source
	Reports []Report
}

// TestStatus represents the status of a mutation test.
type TestStatus int

const (
	// Killed indicates the mutation was detected by tests.
	Killed TestStatus = iota
	// Survived indicates the mutation was not detected by tests.
	Survived
	// Skipped indicates the mutation was skipped.
	Skipped
	// Error indicates an error occurred during testing.
	Error
)

// Result represents the test results for mutations grouped by type.
type Result map[MutationType][]struct {
	MutationID string
	Status     TestStatus
	Err        error
}

// ReportV2 represents the result of testing a mutation in v2 format.
type ReportV2 struct {
	Source SourceV2
	Result Result
}
