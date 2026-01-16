// Package model defines the data structures for mutation testing.
package model

// MutationType represents the category of mutation.
type MutationType string

const (
	// MutationArithmetic represents arithmetic operator mutations (+, -, *, /, %).
	MutationArithmetic MutationType = "arithmetic"
	// MutationBoolean represents boolean literal mutations (true <-> false).
	MutationBoolean MutationType = "boolean"
)

// Mutation represents a code mutation with its details.
// Mutation represents a single mutation applied to source code.
type Mutation struct {
	// ID is the unique identifier for a mutation within a test run.
	ID          int
	Source      Source
	Type        MutationType
	MutatedCode []byte
}
