package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVersionCmd_Output(t *testing.T) {
	cmd := newVersionCmd()

	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	require.NoError(t, err)

	output := out.String()
	if strings.Contains(output, "version: unknown") {
		assert.Contains(t, output, "version: unknown")
		return
	}

	assert.Contains(t, output, "tool version")
	assert.Contains(t, output, "go version")
}
