package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPackageName(t *testing.T) {
	t.Parallel()

	for path, want := range map[string]string{
		"log":                       "log",
		"log/slog":                  "slog",
		"k8s.io/klog/v2":            "klog",
		"example.com/a/b/v10":       "b",
		"v2":                        ".",
		"example.com/vault":         "vault",
		"example.com/v":             "v",
		"example.com/app/v2beta":    "v2beta",
		"example.com/app/2":         "2",
		"github.com/rs/zerolog/log": "log",
	} {
		assert.Equal(t, want, packageName(path), path)
	}
}

func TestAridRanges_Contains(t *testing.T) {
	t.Parallel()

	ranges := aridRanges{{start: 2, end: 4}, {start: 10, end: 11}}

	for offset, want := range map[int]bool{-1: false, 1: false, 2: true, 3: true, 4: false, 9: false, 10: true, 11: false} {
		assert.Equal(t, want, ranges.contains(offset), offset)
	}

	assert.False(t, aridRanges(nil).contains(0))
}

func TestCallPattern(t *testing.T) {
	t.Parallel()

	assert.Equal(t, callPattern{receiver: "example.com/app/audit", function: "*"}, parseCallPattern("example.com/app/audit.*"))
	assert.Equal(t, callPattern{receiver: "", function: "nodot"}, parseCallPattern("nodot"))
	assert.Equal(t, callPattern{receiver: "", function: "x"}, parseCallPattern(".x"))
	assert.Equal(t, callPattern{receiver: "a", function: ""}, parseCallPattern("a."))

	assert.True(t, callPattern{receiver: "*", function: "*"}.matches(nil, "Any"))
	assert.True(t, callPattern{receiver: "a", function: "F"}.matches([]string{"b", "a"}, "F"))
	assert.False(t, callPattern{receiver: "a", function: "F"}.matches([]string{"b"}, "F"))
	assert.False(t, callPattern{receiver: "a", function: "F"}.matches([]string{"a"}, "G"))
	assert.False(t, callPattern{receiver: "a", function: ""}.matches([]string{"a"}, "G"), "an empty function is no wildcard")
	assert.False(t, callPattern{receiver: "", function: "G"}.matches([]string{"a"}, "G"), "an empty receiver is no wildcard")
	assert.False(t, callPattern{receiver: "", function: "G"}.matches(nil, "G"))
}
