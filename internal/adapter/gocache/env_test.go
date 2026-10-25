package gocache

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSupportsCacheProg(t *testing.T) {
	tests := map[string]bool{
		"go1.24.0":                  true,
		"go1.24rc1":                 true,
		"go1.27.3":                  true,
		"go1.100":                   true,
		"devel go1.28-abcdef Mon":   true,
		"go1.23.12":                 false,
		"go1.2":                     false,
		"devel +abcdef":             false,
		"":                          false,
		"go2.0":                     false,
		"go1.":                      false,
		"go1.9999999999999999999.0": false,
	}
	for version, want := range tests {
		assert.Equal(t, want, supportsCacheProg(version), version)
	}
}

func TestFallbackReason(t *testing.T) {
	assert.Empty(t, fallbackReason(goEnv{GOVERSION: "go1.24.0"}))
	assert.Equal(t, "GOCACHEPROG is already set", fallbackReason(goEnv{GOVERSION: "go1.24.0", GOCACHEPROG: "cache"}))
	assert.Equal(t, "GOCACHEPROG needs Go 1.24 or later, not go1.23.4", fallbackReason(goEnv{GOVERSION: "go1.23.4"}))
}

func TestProgCommand(t *testing.T) {
	assert.Equal(t, `"/bin/gooze" __cacheprog --shared "/home/a b/go-build" --layer "/c"`, progCommand("/bin/gooze", "/home/a b/go-build", "/c"))
	assert.Equal(t, `'/x"y/gooze' __cacheprog --shared "/s" --layer "/c"`, progCommand(`/x"y/gooze`, "/s", "/c"))
}
