package config

import (
	"encoding/json"
)

// The JSON Schema words used more than once.
const (
	schemaType   = "type"
	schemaString = "string"
	schemaEnum   = "enum"
)

// JSONSchema returns the JSON Schema of the configuration file, for editors
// to validate and complete gooze.yaml with.
func JSONSchema() []byte {
	schema := objectJSONSchema(Schema())
	schema["$schema"] = "http://json-schema.org/draft-07/schema#"
	schema["title"] = "gooze.yaml"

	// Only strings, numbers, booleans, lists and maps of them: encoding can't fail.
	data, _ := json.MarshalIndent(schema, "", "  ") //nolint:errchkjson // see above.

	return append(data, '\n')
}

func objectJSONSchema(fields []Field) map[string]any {
	properties := make(map[string]any, len(fields))
	for _, field := range fields {
		properties[field.Name] = fieldJSONSchema(field)
	}

	return map[string]any{schemaType: "object", "additionalProperties": false, "properties": properties}
}

func fieldJSONSchema(f Field) map[string]any {
	schema := kindJSONSchema(f)

	if f.Doc != "" {
		schema["description"] = f.Doc
	}

	if f.Default != nil {
		schema["default"] = f.Default
	}

	if f.Min != nil {
		schema["minimum"] = *f.Min
	}

	if f.Max != nil {
		schema["maximum"] = *f.Max
	}

	return schema
}

// kindJSONSchema returns the part of f's schema that its kind decides.
func kindJSONSchema(f Field) map[string]any {
	switch f.Kind {
	case KindObject:
		return objectJSONSchema(f.Children)
	case KindBool:
		return map[string]any{schemaType: "boolean"}
	case KindInt:
		return map[string]any{schemaType: "integer"}
	case KindFloat:
		return map[string]any{schemaType: "number"}
	case KindEnum:
		return enumJSONSchema(f)
	case KindStringList:
		return map[string]any{schemaType: "array", "items": stringItems(f.Enum)}
	default:
		return map[string]any{schemaType: schemaString}
	}
}

// enumJSONSchema lists the values of f. An IntEnum field also takes an integer
// or a listed name in any case, so its list only offers completions.
func enumJSONSchema(f Field) map[string]any {
	if f.IntEnum {
		return map[string]any{"anyOf": []any{map[string]any{schemaEnum: f.Enum}, map[string]any{schemaType: []string{schemaString, "integer"}}}}
	}

	return map[string]any{schemaEnum: f.Enum}
}

func stringItems(enum []string) map[string]any {
	items := map[string]any{schemaType: schemaString}
	if len(enum) > 0 {
		items[schemaEnum] = enum
	}

	return items
}
