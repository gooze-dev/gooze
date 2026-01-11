package model

// Path represents a file system path.
type Path string

// ScopeType defines where code elements can be mutated
type ScopeType string

const (
	// ScopeGlobal represents package-level declarations (const, var, type).
	// Always scanned for mutations like: boolean literals, numbers in consts
	ScopeGlobal ScopeType = "global"

	// ScopeInit represents init() functions.
	// Scanned for all mutation types
	ScopeInit ScopeType = "init"

	// ScopeFunction represents regular function bodies.
	// Scanned for function-specific mutations
	ScopeFunction ScopeType = "function"
)

// CodeScope represents a region of code with its scope type and line range
type CodeScope struct {
	Type      ScopeType
	StartLine int
	EndLine   int
	Name      string // function/variable name for debugging
}

// Source represents a Go source file with mutation scopes.
type Source struct {
	Hash   string
	Origin Path
	Test   Path
	// Lines contains line numbers for backward compatibility (function lines only)
	Lines []int
	// Scopes provides detailed scope information for selective mutation
	Scopes []CodeScope
}
