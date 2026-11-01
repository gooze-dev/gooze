package config

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gooze.dev/pkg/gooze/pkg/config"
)

func TestConfigSchema_PrintsTheJSONSchema(t *testing.T) {
	out := &bytes.Buffer{}
	cmd := New(Deps{Dir: t.TempDir(), FileName: "gooze.yaml"})
	cmd.SetOut(out)
	cmd.SetArgs([]string{"schema"})

	require.NoError(t, cmd.Execute())
	assert.Equal(t, string(config.JSONSchema()), out.String())
}
