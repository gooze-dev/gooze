package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKindString(t *testing.T) {
	want := map[Kind]string{
		KindObject:     "object",
		KindString:     "string",
		KindBool:       "bool",
		KindInt:        "int",
		KindStringList: "string list",
		KindEnum:       "enum",
		KindFloat:      "float",
		Kind(99):       "unknown",
	}

	for kind, name := range want {
		assert.Equal(t, name, kind.String())
	}
}
