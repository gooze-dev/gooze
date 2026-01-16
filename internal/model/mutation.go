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

type Mutation struct {
	ID          uint
	Source      Source
	Type        MutationType
	MutatedCode []byte
}
