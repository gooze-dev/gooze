// Package config describes the schema of gooze's configuration file: every key
// gooze reads, its kind, default value, accepted range and documentation. It
// reads no files; tools such as editors use it to complete and validate
// gooze.yaml.
package config

import (
	"fmt"
	"slices"
	"strings"
)

// Field describes one key of the configuration file.
type Field struct {
	// Name is the key within its parent object, e.g. "parallel" for run.parallel.
	Name string
	// Kind is the type of value the key expects.
	Kind Kind
	// Default is the value gooze uses when the key is not set: a string, bool,
	// int or []string matching Kind, and nil for objects.
	Default any
	// Doc is a one-line description of the key.
	Doc string
	// Enum lists the accepted values of a KindEnum field, or of each item of
	// a KindStringList field (empty means any string).
	Enum []string
	// IntEnum reports whether a KindEnum field also accepts an integer.
	IntEnum bool
	// Min and Max, when non-nil, are the inclusive bounds of a KindInt field's
	// value. CheckInt validates a value against them.
	Min *int
	Max *int
	// Children are the keys of a KindObject field.
	Children []Field
}

// Schema returns the top-level fields of the configuration file in the order
// gooze documents them. Each call returns a new copy.
func Schema() []Field {
	return []Field{
		{Name: KeyVersion, Kind: KindInt, Default: 1, Min: new(1), Max: new(1), Doc: "Config schema version."},
		{Name: KeyOutput, Kind: KindString, Default: DefaultReportsDir, Doc: "Directory where mutation reports are written."},
		{Name: KeyNoCache, Kind: KindBool, Default: false, Doc: "Disable incremental caching and force a full re-run."},
		{Name: KeyNoUpdateCheck, Kind: KindBool, Default: false, Doc: "Disable the check for a newer gooze release."},
		pathsField(),
		runField(),
		logField(),
	}
}

func pathsField() Field {
	return Field{Name: "paths", Kind: KindObject, Doc: "Path selection settings.", Children: []Field{
		{Name: "exclude", Kind: KindStringList, Default: []string{}, Doc: "Regular-expression patterns of files to skip (e.g. '^vendor/')."},
	}}
}

func runField() Field {
	return Field{Name: "run", Kind: KindObject, Doc: "Mutation run settings.", Children: []Field{
		{Name: "parallel", Kind: KindInt, Default: 1, Min: new(1), Doc: "Number of concurrent mutation workers."},
		{Name: "mutation_timeout", Kind: KindInt, Default: 120, Min: new(1), Doc: "Per-mutation timeout in seconds."},
		{Name: "coverage_profile", Kind: KindString, Default: "", Doc: "Go coverage profile; mutations on uncovered lines are reported as not_covered without running tests."},
		{Name: "diff", Kind: KindString, Default: "", Doc: "Git ref to diff against; only lines changed since its merge base with HEAD are mutated. Requires git. Empty runs everything."},
		{
			Name: "mutagens", Kind: KindStringList, Default: []string{},
			Enum: []string{"arithmetic", "boolean", "numbers", "comparison", "logical", "unary"},
			Doc:  "Mutagens to run (arithmetic, boolean, numbers, comparison, logical, unary); empty runs all of them.",
		},
	}}
}

func logField() Field {
	return Field{Name: "log", Kind: KindObject, Doc: "Logging settings.", Children: []Field{
		{Name: "filename", Kind: KindString, Default: ".gooze.log", Doc: "Log file path."},
		{
			Name: "level", Kind: KindEnum, Default: "info", IntEnum: true,
			Enum: []string{"debug", "info", "warn", "warning", "error"},
			Doc:  "Log severity: debug, info, warn or error, or a numeric slog level (-4 debug, 0 info, 4 warn, 8 error).",
		},
		{Name: "verbose", Kind: KindBool, Default: false, Doc: "Enable debug-level logging."},
		{Name: "max_size", Kind: KindInt, Default: 10, Doc: "Max log size in MiB before rotation."},
		{Name: "max_backups", Kind: KindInt, Default: 3, Doc: "Number of rotated log files to retain."},
		{Name: "max_age", Kind: KindInt, Default: 28, Doc: "Days to keep rotated logs."},
		{Name: "compress", Kind: KindBool, Default: true, Doc: "Gzip rotated logs."},
	}}
}

// CheckInt returns an error describing the violated bound when n is outside
// the field's Min and Max, e.g. "0 is out of range: must be at least 1", and nil
// otherwise or when the field has no bounds.
func (f Field) CheckInt(n int) error {
	if (f.Min == nil || n >= *f.Min) && (f.Max == nil || n <= *f.Max) {
		return nil
	}

	return fmt.Errorf("%d is out of range: %s", n, f.boundText())
}

func (f Field) boundText() string {
	switch {
	case f.Min != nil && f.Max != nil && *f.Min == *f.Max:
		return fmt.Sprintf("must be %d", *f.Min)
	case f.Min != nil && f.Max != nil:
		return fmt.Sprintf("must be between %d and %d", *f.Min, *f.Max)
	case f.Min != nil:
		return fmt.Sprintf("must be at least %d", *f.Min)
	default:
		return fmt.Sprintf("must be at most %d", *f.Max)
	}
}

// Lookup returns the field at a dotted key such as "run.parallel".
func Lookup(key string) (Field, bool) {
	field := Field{Kind: KindObject, Children: Schema()}

	for name := range strings.SplitSeq(key, ".") {
		i := slices.IndexFunc(field.Children, func(child Field) bool { return child.Name == name })
		if i < 0 {
			return Field{}, false
		}

		field = field.Children[i]
	}

	return field, true
}
