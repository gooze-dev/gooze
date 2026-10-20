package cmd

import (
	"bytes"
	"runtime/debug"
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

	assert.Regexp(t, `^gooze \S.*\n$`, out.String())
	assert.NotContains(t, out.String(), "\t")
}

func TestVersionLine(t *testing.T) {
	tests := []struct {
		name string
		info debug.BuildInfo
		want string
	}{
		{
			name: "release",
			info: debug.BuildInfo{GoVersion: "go1.27.3", Main: debug.Module{Version: "v1.1.1"}},
			want: "gooze v1.1.1 (go1.27.3)",
		},
		{
			name: "no module version",
			info: debug.BuildInfo{GoVersion: "go1.27.3"},
			want: "gooze (unknown version) (go1.27.3)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, versionLine(&tt.info))
		})
	}
}
