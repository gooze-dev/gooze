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
	// Comparison mutates comparison operators (<, >, <=, >=, ==, !=). Version 2
	// skips the mutant that is equivalent when one operand is a minimum of the
	// other: v == "" is not mutated to v <= "", len(s) != 0 not to len(s) > 0.
	Comparison = Type{Name: "comparison", Version: 2}
	// Logical mutates logical operators (&&, ||).
	Logical = Type{Name: "logical", Version: 1}
	// Unary mutates unary operators (-, +, !, ^).
	Unary = Type{Name: "unary", Version: 1}
	// Bitwise mutates bitwise operators (&, |, ^, &^, <<, >>).
	Bitwise = Type{Name: "bitwise", Version: 1}
	// Increment swaps increment and decrement statements (x++ <-> x--).
	Increment = Type{Name: "increment", Version: 1}
	// Assignment mutates compound assignments (x += y -> x -= y, x += y -> x = y).
	Assignment = Type{Name: "assignment", Version: 1}
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
	// Default reports whether gooze runs the mutagen when no mutagens are
	// selected. The others run only when selected.
	Default bool
}

// All returns every mutagen in registry order. Each call returns a new slice.
func All() []Mutagen {
	return []Mutagen{
		{Type: Arithmetic, Doc: "Mutates arithmetic operators (+, -, *, /, %).", Default: true},
		{Type: Boolean, Doc: "Mutates boolean literals (true <-> false).", Default: true},
		{Type: Numbers, Doc: "Mutates numeric literals (e.g. 5 -> 0, 5 -> 1).", Default: true},
		{Type: Comparison, Doc: `Mutates comparison operators (<, >, <=, >=, ==, !=), except the one equivalent mutant when an operand is "" or 0 compared with len or cap.`, Default: true},
		{Type: Logical, Doc: "Mutates logical operators (&&, ||).", Default: true},
		{Type: Unary, Doc: "Mutates unary operators (-, +, !, ^).", Default: true},
		{Type: Bitwise, Doc: "Mutates bitwise operators (& <-> |, ^ -> &, &^ -> &, << <-> >>).", Default: true},
		{Type: Increment, Doc: "Swaps increment and decrement statements (x++ <-> x--), except in a for loop's post statement.", Default: true},
		{Type: Assignment, Doc: "Mutates compound assignments (+= <-> -=, *= <-> /=, %= -> *=, &= <-> |=, ^= -> &=, &^= -> &=, <<= <-> >>=) and turns x op= y into x = y, except in a for loop's post statement.", Default: true},
		{Type: Branch, Doc: "Inverts and forces if and for conditions, removes if and else blocks and switch case bodies."},
		{Type: Statement, Doc: "Deletes statements (assignments, calls, defer, go, channel sends); declarations with := are kept."},
		{Type: Loop, Doc: "Mutates loops (boundary conditions, body removal, break and continue removal, recursive calls), never so that a loop runs forever."},
	}
}

// Defaults returns the mutagens gooze runs when none are selected, in
// registry order.
func Defaults() []Type {
	var types []Type

	for _, mg := range All() {
		if mg.Default {
			types = append(types, mg.Type)
		}
	}

	return types
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
