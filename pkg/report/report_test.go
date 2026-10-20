package report

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestTestStatus_Values(t *testing.T) {
	assert.Equal(t, TestStatus(0), Killed)
	assert.Equal(t, TestStatus(1), Survived)
	assert.Equal(t, TestStatus(2), Skipped)
	assert.Equal(t, TestStatus(3), Timeout)
	assert.Equal(t, TestStatus(4), Error)
	assert.Equal(t, TestStatus(5), NotCovered)
}

func TestTestStatus_String(t *testing.T) {
	want := map[TestStatus]string{
		Killed:         "killed",
		Survived:       "survived",
		Skipped:        "skipped",
		Timeout:        "timeout",
		Error:          "error",
		NotCovered:     "not_covered",
		TestStatus(42): "unknown",
	}

	for status, name := range want {
		assert.Equal(t, name, status.String())
	}
}

func TestTestStatus_EncodesAsInteger(t *testing.T) {
	data, err := yaml.Marshal(MutationResult{MutationID: "m", Status: NotCovered})
	require.NoError(t, err)
	assert.Equal(t, "mutationid: m\nstatus: 5\n", string(data))
}

func TestSource_Key(t *testing.T) {
	assert.Empty(t, Source{}.Key())
	assert.Equal(t, "/abs/a.go", Source{Origin: &File{FullPath: "/abs/a.go"}}.Key())
	assert.Equal(t, "a.go", Source{Origin: &File{ShortPath: "a.go", FullPath: "/abs/a.go"}}.Key())
}
