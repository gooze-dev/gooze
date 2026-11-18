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
	// Funcs names the function declaration the mutation is in: "Name", plus
	// "Type.Name" for a method. Nil outside any function.
	Funcs []string
	// Ignored is set when a //gooze:ignore directive keeps the mutation from
	// being tested; only a listing of the ignored mutations holds those.
	Ignored *Ignored
	// Guards is where the code starts that only runs through the mutated
	// condition: the first statement of an if or for body, or of a case
	// clause. Nil when the mutation isn't in such a condition.
	Guards *Position
}

// Position is a place in a source file, both counted from 1; the column in
// bytes, as in a Go coverage profile.
type Position struct {
	Line, Column int
}

// Ignored is why a directive ignores a mutation.
type Ignored struct {
	// Reason is the directive's reason=, as written, or "" without one.
	Reason string
}
