// Package model defines the data structures for mutation testing.
package model

import (
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// Mutation represents a code mutation with its details.
// Mutation represents a single mutation applied to source code.
type Mutation struct {
	// ID is the unique identifier for a mutation within a test run.
	ID          string
	Source      rpt.Source
	Type        mgen.Type
	MutatedCode []byte
	DiffCode    []byte
	Line        int
	// Scope is set in diff mode when only some lines of the source are
	// tested; reports of the mutation record it.
	Scope *rpt.Scope
}
