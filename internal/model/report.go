package model

import (
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// Result represents the test results for mutations grouped by type.
type Result map[mgen.Type][]MutationOutcome

// MutationOutcome is the outcome of testing one mutation.
type MutationOutcome struct {
	MutationID string
	Status     rpt.TestStatus
	Err        error
	// KilledBy names the first test that failed on a killed mutation, or ""
	// when the output doesn't name one.
	KilledBy string
	// Rule names the built-in rule that holds an equivalent mutation for one.
	Rule string
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
