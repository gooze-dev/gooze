package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCacheProgCmd_Serves(t *testing.T) {
	root := newRootCmd()
	root.AddCommand(newCacheProgCmd())

	var out bytes.Buffer

	root.SetIn(strings.NewReader(`{"ID":1,"Command":"close"}` + "\n"))
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"__cacheprog", "--shared", t.TempDir(), "--layer", t.TempDir()})

	require.NoError(t, root.Execute())
	assert.Equal(t, `{"ID":0,"KnownCommands":["get","put","close"]}`+"\n"+`{"ID":1}`+"\n", out.String())
	assert.False(t, root.SilenceUsage, "root's pre-run, which logs and checks for updates, is skipped")
}

func TestCacheProgCmd_Hidden(t *testing.T) {
	assert.True(t, newCacheProgCmd().Hidden)
}
