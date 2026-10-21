package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsNewerVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		candidate, current string
		want               bool
	}{
		{"v1.3.0", "v1.2.0", true},
		{"v2.0.0", "v1.9.9", true},
		{"v1.2.1", "v1.2.0", true},
		{"v1.10.0", "v1.9.0", true},
		{"v1.2.0", "v1.2.0", false},
		{"v1.2.0", "v1.3.0", false},
		{"v1.2.0", "v2.0.0", false},
		{"v1.2.0", "v1.2.1", false},
		{"v1.2.0", "v1.2.0-rc.1", true},
		{"v1.2.0-rc.1", "v1.2.0", false},
		{"v1.2.0-rc.2", "v1.2.0-rc.1", true},
		{"v1.2.0-rc.1", "v1.2.0-rc.1", false},
		// A pseudo-version built after v1.2.0 is not behind v1.2.0...
		{"v1.2.0", "v1.2.1-0.20261020134510-ffa805e96336+dirty", false},
		// ...but is behind v1.2.1.
		{"v1.2.1", "v1.2.1-0.20261020134510-ffa805e96336", true},
		{"v1.3.0", "v1.2.0+dirty", true},
		{"", "v1.2.0", false},
		{"v1.3.0", "", false},
		{"v1.3.0", "(devel)", false},
		{"1.3.0", "v1.2.0", false},
		{"v1.3", "v1.2.0", false},
		{"v1.3.0.1", "v1.2.0", false},
		{"v1.x.0", "v1.2.0", false},
		{"v1.-3.0", "v1.2.0", false},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, IsNewerVersion(tt.candidate, tt.current), "IsNewerVersion(%q, %q)", tt.candidate, tt.current)
	}
}

func TestNoticeFor(t *testing.T) {
	t.Parallel()

	notice, ok := NoticeFor("v1.2.0", "v1.3.0")
	assert.True(t, ok)
	assert.Equal(t, UpdateNotice{Current: "v1.2.0", Latest: "v1.3.0"}, notice)

	for _, tc := range [][2]string{{"v1.2.0", "v1.2.0"}, {"v1.2.0", ""}, {"(devel)", "v1.3.0"}, {"", "v1.3.0"}} {
		_, ok := NoticeFor(tc[0], tc[1])
		assert.False(t, ok, "NoticeFor(%q, %q)", tc[0], tc[1])
	}
}

func TestReleased(t *testing.T) {
	t.Parallel()

	assert.True(t, Released("v1.2.0"))
	assert.True(t, Released("v1.2.1-0.20261020134510-ffa805e96336+dirty"))
	assert.False(t, Released(""))
	assert.False(t, Released("(devel)"))
}

func TestUpdateNotice_String(t *testing.T) {
	t.Parallel()

	got := UpdateNotice{Current: "v1.2.0", Latest: "v1.3.0"}.String()
	assert.Equal(t, "gooze v1.3.0 is available (you have v1.2.0). Update: go install gooze.dev/pkg/gooze@latest", got)
}
