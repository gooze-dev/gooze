package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaults(t *testing.T) {
	want := map[string]any{
		KeyVersion:            1,
		KeyOutput:             DefaultReportsDir,
		KeyNoCache:            false,
		KeyPathsExclude:       []string{},
		KeyRunParallel:        1,
		KeyRunMutationTimeout: 120,
		KeyRunCoverageProfile: "",
		KeyLogFilename:        ".gooze.log",
		KeyLogLevel:           0,
		KeyLogVerbose:         false,
		KeyLogMaxSize:         10,
		KeyLogMaxBackups:      3,
		KeyLogMaxAge:          28,
		KeyLogCompress:        true,
	}

	for _, key := range Keys() {
		field, ok := Lookup(key)
		require.True(t, ok, key)
		assert.Equal(t, want[key], field.Default, key)
	}
}

func TestLookup_Leaf(t *testing.T) {
	field, ok := Lookup("run.parallel")
	require.True(t, ok)
	assert.Equal(t, "parallel", field.Name)
	assert.Equal(t, KindInt, field.Kind)
	assert.Equal(t, 1, field.Default)
	assert.NotEmpty(t, field.Doc)
}

func TestLookup_Object(t *testing.T) {
	field, ok := Lookup("run")
	require.True(t, ok)
	assert.Equal(t, KindObject, field.Kind)
	assert.Nil(t, field.Default)

	names := make([]string, 0, len(field.Children))
	for _, child := range field.Children {
		names = append(names, child.Name)
	}

	assert.Equal(t, []string{"parallel", "mutation_timeout", "coverage_profile"}, names)
}

func TestLookup_Missing(t *testing.T) {
	for _, key := range []string{"", "nope", "run.nope", "run.parallel.extra", "version.x", ".run", "run."} {
		_, ok := Lookup(key)
		assert.False(t, ok, key)
	}
}

func TestLookup_LogLevelEnum(t *testing.T) {
	field, ok := Lookup(KeyLogLevel)
	require.True(t, ok)
	assert.Equal(t, KindEnum, field.Kind)
	assert.True(t, field.IntEnum)
	assert.Equal(t, []string{"debug", "info", "warn", "warning", "error"}, field.Enum)
}

func TestLookup_Kinds(t *testing.T) {
	want := map[string]Kind{
		KeyVersion:            KindInt,
		KeyOutput:             KindString,
		KeyNoCache:            KindBool,
		"paths":               KindObject,
		KeyPathsExclude:       KindStringList,
		KeyRunMutationTimeout: KindInt,
		KeyRunCoverageProfile: KindString,
		"log":                 KindObject,
		KeyLogCompress:        KindBool,
	}

	for key, kind := range want {
		field, ok := Lookup(key)
		require.True(t, ok, key)
		assert.Equal(t, kind, field.Kind, key)
	}
}

func TestSchema_EveryFieldDocumented(t *testing.T) {
	var walk func(fields []Field)

	walk = func(fields []Field) {
		for _, field := range fields {
			assert.NotEmpty(t, field.Name)
			assert.NotEmpty(t, field.Doc, field.Name)
			assert.Equal(t, field.Kind == KindObject, len(field.Children) > 0, field.Name)
			walk(field.Children)
		}
	}

	walk(Schema())
}

func TestSchema_ReturnsCopy(t *testing.T) {
	first := Schema()
	first[0].Name = "changed"
	first[3].Children[0].Name = "changed"

	second := Schema()
	assert.Equal(t, "version", second[0].Name)
	assert.Equal(t, "exclude", second[3].Children[0].Name)
}
