// Package mutagen is the registry of gooze's mutagens: the categories of
// mutations gooze generates, their versions and their documentation. Names are
// the lower-case identifiers used in reports and in //gooze:ignore directives.
package mutagen

import "strings"

// Type identifies a mutagen and the version of its rules. Reports store both;
// gooze re-tests a source when the version of a mutagen changes.
type Type struct {
	Name    string
	Version int
}

// String returns the name of the mutagen.
func (t Type) String() string {
	return t.Name
}

var (
	// Arithmetic mutates arithmetic operators (+, -, *, /, %).
	Arithmetic = Type{Name: "arithmetic", Version: 1}
	// Boolean mutates boolean literals (true <-> false).
	Boolean = Type{Name: "boolean", Version: 1}
	// Numbers mutates numeric literals (e.g. 5 -> 0, 5 -> 1).
	Numbers = Type{Name: "numbers", Version: 1}
	// Comparison mutates comparison operators (<, >, <=, >=, ==, !=).
	Comparison = Type{Name: "comparison", Version: 1}
	// Logical mutates logical operators (&&, ||).
	Logical = Type{Name: "logical", Version: 1}
	// Unary mutates unary operators (-, +, !, ^).
	Unary = Type{Name: "unary", Version: 1}
	// Branch mutates branch and conditional expressions (if, for, switch conditions).
	Branch = Type{Name: "branch", Version: 1}
	// Statement deletes statements (assignments, expressions, defer, go, send).
	Statement = Type{Name: "statement", Version: 1}
	// Loop mutates loops (boundary conditions, loop body removal, break/continue removal).
	Loop = Type{Name: "loop", Version: 1}
)

// Mutagen describes a mutagen for users.
type Mutagen struct {
	Type
	// Doc is a one-line description of the mutations the mutagen generates.
	Doc string
}

// All returns every mutagen in registry order. Each call returns a new slice.
func All() []Mutagen {
	return []Mutagen{
		{Type: Arithmetic, Doc: "Mutates arithmetic operators (+, -, *, /, %)."},
		{Type: Boolean, Doc: "Mutates boolean literals (true <-> false)."},
		{Type: Numbers, Doc: "Mutates numeric literals (e.g. 5 -> 0, 5 -> 1)."},
		{Type: Comparison, Doc: "Mutates comparison operators (<, >, <=, >=, ==, !=)."},
		{Type: Logical, Doc: "Mutates logical operators (&&, ||)."},
		{Type: Unary, Doc: "Mutates unary operators (-, +, !, ^)."},
		{Type: Branch, Doc: "Mutates branch and conditional expressions (if, for, switch conditions)."},
		{Type: Statement, Doc: "Deletes statements (assignments, expressions, defer, go, send)."},
		{Type: Loop, Doc: "Mutates loops (boundary conditions, loop body removal, break/continue removal)."},
	}
}

// Lookup returns the mutagen with the given name, ignoring case and
// surrounding whitespace as gooze does in directives.
func Lookup(name string) (Mutagen, bool) {
	name = strings.ToLower(strings.TrimSpace(name))

	for _, mg := range All() {
		if mg.Name == name {
			return mg, true
		}
	}

	return Mutagen{}, false
}

// IsValid reports whether name is a known mutagen, see Lookup.
func IsValid(name string) bool {
	_, ok := Lookup(name)
	return ok
}

// Names returns the name of every mutagen in registry order.
func Names() []string {
	all := All()

	names := make([]string, 0, len(all))
	for _, mg := range all {
		names = append(names, mg.Name)
	}

	return names
}

// Versions returns the current version of every mutagen by name.
func Versions() map[string]int {
	all := All()

	versions := make(map[string]int, len(all))
	for _, mg := range all {
		versions[mg.Name] = mg.Version
	}

	return versions
}
