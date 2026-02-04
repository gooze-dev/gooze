package model

// TestStatus represents the status of a mutation test.
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
	// Error indicates an error occurred during testing.
	Error
)

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
	default:
		return "unknown"
	}
}

// Result represents the test results for mutations grouped by type.
type Result map[MutationType][]struct {
	MutationID string
	Status     TestStatus
	Err        error
}

// Report represents the result of testing a mutation source file.
type Report struct {
	Source Source
	Result Result
	Diff   *[]byte
}
