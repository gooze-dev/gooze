package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// lookupPath returns the value at a dotted key in a decoded YAML document.
func lookupPath(t *testing.T, doc map[string]any, key string) any {
	t.Helper()

	parts := strings.Split(key, ".")
	node := doc

	for _, part := range parts[:len(parts)-1] {
		child, ok := node[part].(map[string]any)
		require.Truef(t, ok, "%s: %s is not an object", key, part)

		node = child
	}

	value, ok := node[parts[len(parts)-1]]
	require.Truef(t, ok, "%s is missing", key)

	return value
}

func TestDefaultFile_HoldsEveryKeyWithItsDefault(t *testing.T) {
	var doc map[string]any
	require.NoError(t, yaml.Unmarshal(DefaultFile(), &doc))

	for _, key := range Keys() {
		field, ok := Lookup(key)
		require.True(t, ok)

		if field.Unset {
			continue // see TestDefaultFile_LeavesUnsetKeysCommentedOut
		}

		got := lookupPath(t, doc, key)

		if field.Init != nil {
			assert.Equal(t, field.Init, got, key)
			continue
		}

		switch field.Kind {
		case KindStringList:
			assert.Empty(t, got, key)
			assert.IsType(t, []any{}, got, key)
		case KindFloat:
			// YAML reads a whole-number default such as 0 back as an int.
			assert.EqualValues(t, field.Default, got, key)
		default:
			assert.Equal(t, field.Default, got, key)
		}
	}
}

func TestDefaultFile_Layout(t *testing.T) {
	lines := strings.Split(strings.TrimSuffix(string(DefaultFile()), "\n"), "\n")

	require.Equal(t, "# gooze.yaml: every key is optional; these are the defaults, unless a comment gives another.", lines[0])
	assert.Contains(t, lines, "  mutation_memory: 1024       # Memory limit of a mutation's tests in MiB, all their processes together; above it they are killed and the mutation is reported as a timeout. 0 is no limit. Linux only. Default: 0.")
	assert.Contains(t, lines, "no_cache: false               # Disable incremental caching and force a full re-run.")
	assert.Contains(t, lines, "")
	assert.Contains(t, lines, "run:                          # Mutation run settings.")
	assert.Contains(t, lines, "  parallel: 1                 # Number of concurrent mutation workers.")
	assert.Contains(t, lines, "    total: 0                  # Minimum for the changed lines' score.")
	assert.Contains(t, lines, "  level: info                 # Log severity: debug, info, warn or error, or a numeric slog level (-4 debug, 0 info, 4 warn, 8 error).")

	// Sections follow schema order, each after one blank line.
	text := string(DefaultFile())
	assert.Less(t, strings.Index(text, "\n\npaths:"), strings.Index(text, "\n\nrun:"))
	assert.Less(t, strings.Index(text, "\n\nrun:"), strings.Index(text, "\n\nthreshold:"))
	assert.Less(t, strings.Index(text, "\n\nthreshold:"), strings.Index(text, "\n\nlog:"))
	assert.NotContains(t, text, "\n\n\n")
	assert.NotContains(t, text, "\n\n  ", "nested objects get no blank line")
}

func TestRenderLine(t *testing.T) {
	assert.Equal(t, "a: 1"+strings.Repeat(" ", commentColumn-4)+"# doc\n", renderLine("a: 1", "doc"))

	// One column short of the comment column still gets its one space.
	short := strings.Repeat("x", commentColumn-1)
	assert.Equal(t, short+" # doc\n", renderLine(short, "doc"))

	long := strings.Repeat("x", commentColumn+5)
	assert.Equal(t, long+" # doc\n", renderLine(long, "doc"))
	assert.Equal(t, "a:\n", renderLine("a:", ""))
}

func TestYAMLValue(t *testing.T) {
	tests := []struct {
		value any
		want  string
	}{
		{true, "true"},
		{false, "false"},
		{120, "120"},
		{0.0, "0"},
		{87.5, "87.5"},
		{"", "''"},
		{".gooze-reports", ".gooze-reports"},
		{"overlay", "overlay"},
		{"deep-copy", "deep-copy"},
		{"true", `"true"`},
		{"No", `"No"`},
		{"^vendor/", `"^vendor/"`},
		{"a b", `"a b"`},
		{"9lives", `"9lives"`},
		{[]string{}, "[]"},
		{[]string{"comparison", "a b"}, `[comparison, "a b"]`},
	}

	for _, tt := range tests {
		got := yamlValue(tt.value)
		assert.Equal(t, tt.want, got, "%#v", tt.value)

		// Each rendering reads back as the same value.
		var decoded struct{ V any }
		require.NoError(t, yaml.Unmarshal([]byte("v: "+got), &decoded))

		switch want := tt.value.(type) {
		case []string:
			assert.Len(t, decoded.V, len(want))
		case float64:
			assert.EqualValues(t, want, decoded.V)
		default:
			assert.Equal(t, tt.value, decoded.V)
		}
	}
}

func TestYAMLValue_PanicsOnUnknownType(t *testing.T) {
	assert.PanicsWithValue(t, "config: no YAML form for default 1.5", func() { yamlValue(float32(1.5)) })
}

func TestDefaultFile_LeavesUnsetKeysCommentedOut(t *testing.T) {
	var doc map[string]any
	require.NoError(t, yaml.Unmarshal(DefaultFile(), &doc))

	oci, ok := lookupPath(t, doc, "report.oci.compat_hosts").([]any)
	require.True(t, ok)
	assert.Empty(t, oci)

	report, _ := doc["report"].(map[string]any)
	_, set := report["oci"].(map[string]any)["compat"]
	assert.False(t, set, "report.oci.compat unset means automatic, which no value says")

	assert.Contains(t, string(DefaultFile()), "\n    # compat: false ", "written commented out, to show the key")
}
