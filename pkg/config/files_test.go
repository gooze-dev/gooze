package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFileNames(t *testing.T) {
	assert.Equal(t, "gooze.yaml", FileName)
	assert.Equal(t, []string{"gooze.yaml"}, FileNames())
	assert.Equal(t, ".gooze-reports", DefaultReportsDir)
}

func TestIsConfigFile(t *testing.T) {
	cases := map[string]bool{
		"gooze.yaml":     true,
		"gooze.yml":      false,
		".gooze.yaml":    false,
		".gooze.yml":     false,
		"dir/gooze.yaml": false,
		"":               false,
	}

	for name, want := range cases {
		assert.Equal(t, want, IsConfigFile(name), name)
	}
}
