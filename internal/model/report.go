package model

import (
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// Result represents the test results for mutations grouped by type.
type Result map[mgen.Type][]struct {
	MutationID string
	Status     rpt.TestStatus
	Err        error
}

// Report represents the result of testing a mutation source file.
type Report struct {
	Source rpt.Source
	Result Result
	Diff   *[]byte
	// Line is the source line of the mutation; 0 when unknown (old reports).
	Line int
	// Scope is set when only some lines of the source were tested.
	Scope *rpt.Scope
}
