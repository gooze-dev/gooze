// Package model defines the data structures for mutation testing.
package model

import "go/token"

// MutationType represents the category of mutation.
type MutationType string

const (
	// MutationArithmetic represents arithmetic operator mutations (+, -, *, /, %).
	MutationArithmetic MutationType = "arithmetic"
	// MutationBoolean represents boolean literal mutations (true <-> false).
	MutationBoolean MutationType = "boolean"
)

// Mutation represents a code mutation for testing.
type Mutation struct {
	ID           string
	Type         MutationType
	SourceFile   Path
	OriginalOp   token.Token
	MutatedOp    token.Token
	OriginalText string // For identifier-based mutations (e.g., "true" -> "false")
	MutatedText  string // For identifier-based mutations (e.g., "false" -> "true")
	Line         int
	Column       int
	ScopeType    ScopeType
}

type MutationV2 struct {
	ID          uint
	Source      SourceV2
	Type        MutationType
	MutatedCode []byte
}
