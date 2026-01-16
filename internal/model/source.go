package model

// Path represents a file system path.
type Path string

// ScopeType defines where code elements can be mutated.
type ScopeType string

const (
	// ScopeGlobal represents package-level declarations (const, var, type).
	// Always scanned for mutations like: boolean literals, numbers in consts.
	ScopeGlobal ScopeType = "global"

	// ScopeInit represents init() functions.
	// Scanned for all mutation types.
	ScopeInit ScopeType = "init"

	// ScopeFunction represents regular function bodies.
	// Scanned for function-specific mutations.
	ScopeFunction ScopeType = "function"
)

// File represents a source code file.
type File struct {
	Path Path
	Hash string
}

// Source represents a pair of source and test files along with their package name.
// Source represents a Go source file and its optional test file metadata.
type Source struct {
	Origin  *File
	Test    *File
	Package *string
}
