// Package config describes the schema of gooze's configuration file: every key
// gooze reads, its kind, default value and documentation. It reads no files;
// tools such as editors use it to complete and validate gooze.yaml.
package config

import (
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
	// Enum lists the accepted values of a KindEnum field.
	Enum []string
	// IntEnum reports whether a KindEnum field also accepts an integer.
	IntEnum bool
	// Children are the keys of a KindObject field.
	Children []Field
}

// Schema returns the top-level fields of the configuration file in the order
// gooze documents them. Each call returns a new copy.
func Schema() []Field {
	return []Field{
		{Name: KeyVersion, Kind: KindInt, Default: 1, Doc: "Config schema version."},
		{Name: KeyOutput, Kind: KindString, Default: DefaultReportsDir, Doc: "Directory where mutation reports are written."},
		{Name: KeyNoCache, Kind: KindBool, Default: false, Doc: "Disable incremental caching and force a full re-run."},
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
		{Name: "parallel", Kind: KindInt, Default: 1, Doc: "Number of concurrent mutation workers."},
		{Name: "mutation_timeout", Kind: KindInt, Default: 120, Doc: "Per-mutation timeout in seconds."},
		{Name: "coverage_profile", Kind: KindString, Default: "", Doc: "Go coverage profile; mutations on uncovered lines are reported as not_covered without running tests."},
	}}
}

func logField() Field {
	return Field{Name: "log", Kind: KindObject, Doc: "Logging settings.", Children: []Field{
		{Name: "filename", Kind: KindString, Default: ".gooze.log", Doc: "Log file path."},
		{
			Name: "level", Kind: KindEnum, Default: 0, IntEnum: true,
			Enum: []string{"debug", "info", "warn", "warning", "error"},
			Doc:  "Log severity: debug, info, warn or error, or a numeric slog level (0 is info).",
		},
		{Name: "verbose", Kind: KindBool, Default: false, Doc: "Enable debug-level logging."},
		{Name: "max_size", Kind: KindInt, Default: 10, Doc: "Max log size in MiB before rotation."},
		{Name: "max_backups", Kind: KindInt, Default: 3, Doc: "Number of rotated log files to retain."},
		{Name: "max_age", Kind: KindInt, Default: 28, Doc: "Days to keep rotated logs."},
		{Name: "compress", Kind: KindBool, Default: true, Doc: "Gzip rotated logs."},
	}}
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
