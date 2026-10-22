package config

import (
	"bytes"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// commentColumn is where the trailing "# doc" comment of each line starts,
// so the comments of a file line up.
const commentColumn = 30

// DefaultFile returns a gooze.yaml holding every key of the schema with its
// default value and its documentation as a trailing comment, in schema order.
// It depends on nothing but the schema: no environment variable, flag or
// existing file changes it.
func DefaultFile() []byte {
	var out bytes.Buffer

	out.WriteString("# gooze.yaml: every key is optional; these are the defaults.\n")
	out.WriteString("# Reference: https://gooze.dev/docs/config/\n")

	writeFields(&out, Schema(), 0)

	return out.Bytes()
}

func writeFields(out *bytes.Buffer, fields []Field, depth int) {
	indent := strings.Repeat("  ", depth)

	for _, field := range fields {
		// Equivalent: KindObject is the lowest Kind, so == is <=.
		if field.Kind == KindObject { //gooze:ignore comparison
			// A blank line sets each top-level section apart.
			// Equivalent: depth starts at 0 and only grows, so == 0 is <= 0.
			if depth == 0 { //gooze:ignore comparison
				out.WriteString("\n")
			}

			writeLine(out, indent+field.Name+":", field.Doc)
			writeFields(out, field.Children, depth+1)

			continue
		}

		writeLine(out, indent+field.Name+": "+yamlValue(field.Default), field.Doc)
	}
}

func writeLine(out *bytes.Buffer, code, doc string) {
	out.WriteString(renderLine(code, doc))
}

// renderLine returns code with doc as a comment starting at commentColumn,
// or one space after code when it is longer.
func renderLine(code, doc string) string {
	if doc == "" {
		return code + "\n"
	}

	return code + strings.Repeat(" ", max(1, commentColumn-len(code))) + "# " + doc + "\n"
}

// yamlValue renders a schema default as a YAML value of the right type:
// numbers and booleans plain, strings quoted only when YAML needs it, lists in
// flow style.
func yamlValue(value any) string {
	switch v := value.(type) {
	case bool:
		return strconv.FormatBool(v)
	case int:
		return strconv.Itoa(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case string:
		return yamlString(v)
	case []string:
		items := make([]string, 0, len(v))
		for _, item := range v {
			items = append(items, yamlString(item))
		}

		return "[" + strings.Join(items, ", ") + "]"
	default:
		panic(fmt.Sprintf("config: no YAML form for default %#v", value))
	}
}

// plainString matches strings YAML reads back as the same string without
// quotes; yamlKeywords are the plain words it would read as something else.
var (
	plainString  = regexp.MustCompile(`^[A-Za-z_./][A-Za-z0-9_./-]*$`)
	yamlKeywords = []string{"true", "false", "yes", "no", "on", "off", "y", "n", "null"}
)

func yamlString(s string) string {
	if s == "" {
		return "''"
	}

	if plainString.MatchString(s) && !slices.Contains(yamlKeywords, strings.ToLower(s)) {
		return s
	}

	// A Go-quoted string is a valid YAML double-quoted string.
	return strconv.Quote(s)
}
