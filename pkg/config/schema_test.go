package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaults(t *testing.T) {
	want := map[string]any{
		KeyVersion:                     1,
		KeyOutput:                      DefaultReportsDir,
		KeyNoCache:                     false,
		KeyNoUpdateCheck:               false,
		KeyPathsExclude:                []string{},
		KeyRunParallel:                 1,
		KeyRunMutationTimeout:          120,
		KeyRunCoverageProfile:          "",
		KeyRunDiff:                     "",
		KeyRunMutagens:                 []string{},
		KeyRunWorkspace:                "overlay",
		KeyMutagenComparisonEnumBounds: false,
		KeyThresholdTotal:              0.0,
		KeyThresholdAverage:            0.0,
		KeyThresholdFile:               0.0,
		KeyThresholdDiffTotal:          0.0,
		KeyThresholdDiffAverage:        0.0,
		KeyThresholdDiffFile:           0.0,
		KeyReportFormat:                "text",
		KeyLogFilename:                 ".gooze.log",
		KeyLogLevel:                    "info",
		KeyLogVerbose:                  false,
		KeyLogMaxSize:                  10,
		KeyLogMaxBackups:               3,
		KeyLogMaxAge:                   28,
		KeyLogCompress:                 true,
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

	assert.Equal(t, []string{"parallel", "mutation_timeout", "coverage_profile", "diff", "mutagens", "workspace"}, names)
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
		KeyRunDiff:            KindString,
		KeyRunMutagens:        KindStringList,
		KeyRunWorkspace:       KindEnum,
		"threshold":           KindObject,
		KeyThresholdTotal:     KindFloat,
		"threshold.diff":      KindObject,
		KeyThresholdDiffFile:  KindFloat,
		"report":              KindObject,
		KeyReportFormat:       KindEnum,
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
	first[4].Children[0].Name = "changed"

	second := Schema()
	assert.Equal(t, "version", second[0].Name)
	assert.Equal(t, "exclude", second[4].Children[0].Name)
}

func TestSchema_Bounds(t *testing.T) {
	want := map[string][2]*int{
		KeyVersion:            {new(1), new(1)},
		KeyRunParallel:        {new(1), nil},
		KeyRunMutationTimeout: {new(1), nil},
	}

	for _, key := range thresholdKeys {
		want[key] = [2]*int{new(0), new(100)}
	}

	for _, key := range Keys() {
		field, ok := Lookup(key)
		require.True(t, ok, key)

		bounds := want[key]
		assert.Equal(t, bounds[0], field.Min, key)
		assert.Equal(t, bounds[1], field.Max, key)

		switch {
		case field.Kind == KindFloat:
			assert.NoError(t, field.CheckFloat(field.Default.(float64)), key)
		case field.Min != nil || field.Max != nil:
			assert.Equal(t, KindInt, field.Kind, key)
			assert.NoError(t, field.CheckInt(field.Default.(int)), key)
		}
	}
}

func TestField_CheckInt(t *testing.T) {
	tests := []struct {
		name    string
		field   Field
		value   int
		wantErr string
	}{
		{name: "no bounds", field: Field{}, value: -5},
		{name: "at min", field: Field{Min: new(1)}, value: 1},
		{name: "below min", field: Field{Min: new(1)}, value: 0, wantErr: "0 is out of range: must be at least 1"},
		{name: "at max", field: Field{Max: new(3)}, value: 3},
		{name: "above max", field: Field{Max: new(3)}, value: 4, wantErr: "4 is out of range: must be at most 3"},
		{name: "inside range", field: Field{Min: new(1), Max: new(3)}, value: 2},
		{name: "outside range", field: Field{Min: new(1), Max: new(3)}, value: 9, wantErr: "9 is out of range: must be between 1 and 3"},
		{name: "inverted bounds", field: Field{Min: new(3), Max: new(1)}, value: 2, wantErr: "2 is out of range: must be between 3 and 1"},
		{name: "single value", field: Field{Min: new(1), Max: new(1)}, value: 2, wantErr: "2 is out of range: must be 1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.field.CheckInt(tt.value)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}

			assert.EqualError(t, err, tt.wantErr)
		})
	}
}

func TestRunMutagensListsItsValues(t *testing.T) {
	field, ok := Lookup(KeyRunMutagens)
	require.True(t, ok)
	assert.Equal(t, KindStringList, field.Kind)
	assert.Equal(t, []string{
		"arithmetic", "boolean", "numbers", "comparison", "logical", "unary",
		"bitwise", "increment", "assignment", "branch", "statement", "loop",
	}, field.Enum, "each item of run.mutagens must be one of these")
	assert.Equal(t, "Mutagens to run (arithmetic, boolean, numbers, comparison, logical, unary, bitwise, increment, assignment, branch, statement, loop); empty runs the default ones, all but branch, statement and loop.", field.Doc)
}

func TestReportFormatListsItsValues(t *testing.T) {
	field, ok := Lookup(KeyReportFormat)
	require.True(t, ok)
	assert.Equal(t, KindEnum, field.Kind)
	assert.False(t, field.IntEnum)
	assert.Equal(t, "text", field.Default)
	assert.Equal(t, []string{"text", "github", "gitlab", "sarif", "codeclimate"}, field.Enum)
}

func TestRunWorkspaceListsItsValues(t *testing.T) {
	field, ok := Lookup(KeyRunWorkspace)
	require.True(t, ok)
	assert.Equal(t, KindEnum, field.Kind)
	assert.False(t, field.IntEnum)
	assert.Equal(t, "overlay", field.Default)
	assert.Equal(t, []string{"overlay", "deep-copy"}, field.Enum)
}

var thresholdKeys = []string{
	KeyThresholdTotal, KeyThresholdAverage, KeyThresholdFile,
	KeyThresholdDiffTotal, KeyThresholdDiffAverage, KeyThresholdDiffFile,
}

func TestField_CheckFloat(t *testing.T) {
	percent := Field{Min: new(0), Max: new(100)}

	tests := []struct {
		name    string
		field   Field
		value   float64
		wantErr string
	}{
		{name: "no bounds", field: Field{}, value: -5.5},
		{name: "at min", field: percent, value: 0},
		{name: "just below min", field: percent, value: -0.01, wantErr: "-0.01 is out of range: must be between 0 and 100"},
		{name: "decimal inside", field: percent, value: 87.5},
		{name: "at max", field: percent, value: 100},
		{name: "just above max", field: percent, value: 100.01, wantErr: "100.01 is out of range: must be between 0 and 100"},
		{name: "min only", field: Field{Min: new(1)}, value: 0.5, wantErr: "0.5 is out of range: must be at least 1"},
		{name: "max only", field: Field{Max: new(3)}, value: 3.5, wantErr: "3.5 is out of range: must be at most 3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.field.CheckFloat(tt.value)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}

			assert.EqualError(t, err, tt.wantErr)
		})
	}
}
