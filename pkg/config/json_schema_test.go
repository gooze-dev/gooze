package config

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFieldJSONSchema(t *testing.T) {
	low, high := 1, 9

	for name, tt := range map[string]struct {
		field Field
		want  map[string]any
	}{
		"string": {Field{Kind: KindString, Doc: "d", Default: "x"}, map[string]any{"type": "string", "description": "d", "default": "x"}},
		"bool":   {Field{Kind: KindBool, Default: false}, map[string]any{"type": "boolean", "default": false}},
		"int":    {Field{Kind: KindInt, Default: 2}, map[string]any{"type": "integer", "default": 2}},
		"bounded int": {
			Field{Kind: KindInt, Default: 2, Min: &low, Max: &high},
			map[string]any{"type": "integer", "default": 2, "minimum": 1, "maximum": 9},
		},
		"float": {
			Field{Kind: KindFloat, Default: 0.5, Min: &low},
			map[string]any{"type": "number", "default": 0.5, "minimum": 1},
		},
		"enum": {
			Field{Kind: KindEnum, Default: "a", Enum: []string{"a", "b"}},
			map[string]any{"enum": []string{"a", "b"}, "default": "a"},
		},
		"int enum": {
			Field{Kind: KindEnum, Default: "a", Enum: []string{"a"}, IntEnum: true},
			map[string]any{
				"anyOf":   []any{map[string]any{"enum": []string{"a"}}, map[string]any{"type": []string{"string", "integer"}}},
				"default": "a",
			},
		},
		"list": {
			Field{Kind: KindStringList, Default: []string{}},
			map[string]any{"type": "array", "default": []string{}, "items": map[string]any{"type": "string"}},
		},
		"enum list": {
			Field{Kind: KindStringList, Default: []string{}, Enum: []string{"a"}},
			map[string]any{"type": "array", "default": []string{}, "items": map[string]any{"type": "string", "enum": []string{"a"}}},
		},
		"object": {
			Field{Kind: KindObject, Doc: "o", Children: []Field{{Name: "c", Kind: KindBool, Default: true}}},
			map[string]any{
				"type": "object", "description": "o", "additionalProperties": false,
				"properties": map[string]any{"c": map[string]any{"type": "boolean", "default": true}},
			},
		},
	} {
		assert.Equal(t, tt.want, fieldJSONSchema(tt.field), name)
	}
}

func TestJSONSchema_DescribesTheFile(t *testing.T) {
	data := JSONSchema()
	require.True(t, strings.HasSuffix(string(data), "}\n"))

	var schema map[string]any
	require.NoError(t, json.Unmarshal(data, &schema))

	assert.Equal(t, "http://json-schema.org/draft-07/schema#", schema["$schema"])
	assert.Equal(t, "gooze.yaml", schema["title"])
	assert.Equal(t, "object", schema["type"])
	assert.Equal(t, false, schema["additionalProperties"])

	for _, key := range Keys() {
		node := schema
		for part := range strings.SplitSeq(key, ".") {
			properties, ok := node["properties"].(map[string]any)
			require.True(t, ok, key)

			node, ok = properties[part].(map[string]any)
			require.True(t, ok, key)
		}

		assert.NotEmpty(t, node["description"], key)
	}
}
