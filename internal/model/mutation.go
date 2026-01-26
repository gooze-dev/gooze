// Package model defines the data structures for mutation testing.
package model

// MutationType represents the category of mutation.
type MutationType struct {
	Name    string
	Version int
}

// String returns the string representation of the mutation type.
func (mt MutationType) String() string {
	return mt.Name
}

var (
	// MutationArithmetic represents arithmetic operator mutations (+, -, *, /, %).
	MutationArithmetic = MutationType{Name: "arithmetic", Version: 1}
	// MutationBoolean represents boolean literal mutations (true <-> false).
	MutationBoolean = MutationType{Name: "boolean", Version: 1}
	// MutationNumbers represents numeric literal mutations (e.g. 5 -> 0, 5 -> 1).
	MutationNumbers = MutationType{Name: "numbers", Version: 1}
	// MutationComparison represents comparison operator mutations (<, >, <=, >=, ==, !=).
	MutationComparison = MutationType{Name: "comparison", Version: 1}
	// MutationLogical represents logical operator mutations (&&, ||).
	MutationLogical = MutationType{Name: "logical", Version: 1}
	// MutationUnary represents unary operator mutations (-, +, !, ^).
	MutationUnary = MutationType{Name: "unary", Version: 1}
)

// Mutation represents a code mutation with its details.
// Mutation represents a single mutation applied to source code.
type Mutation struct {
	// ID is the unique identifier for a mutation within a test run.
	ID          string
	Source      Source
	Type        MutationType
	MutatedCode []byte
	DiffCode    []byte
}
