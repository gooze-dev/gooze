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

func TestReport_LineAndScopeAreOptional(t *testing.T) {
	// Reports written before diff mode have neither field, and full results
	// still omit them, so old and new files look the same.
	data, err := yaml.Marshal(Report{Source: Source{}})
	require.NoError(t, err)
	assert.NotContains(t, string(data), "line")
	assert.NotContains(t, string(data), "scope")

	var old Report
	require.NoError(t, yaml.Unmarshal([]byte("source: {}\nresult: []\ndiff: null\n"), &old))
	assert.Zero(t, old.Line)
	assert.Nil(t, old.Scope)
}

func TestReport_ScopeRoundTrip(t *testing.T) {
	in := Report{
		Result: []Result{},
		Line:   12,
		Scope:  &Scope{Base: "1a2b3c4", Lines: []LineRange{{Start: 3, End: 3}, {Start: 10, End: 12}}},
	}

	data, err := yaml.Marshal(in)
	require.NoError(t, err)
	assert.Contains(t, string(data), "line: 12\n")
	assert.Contains(t, string(data), "scope:\n    base: 1a2b3c4\n    lines:\n        - start: 3\n          end: 3\n")

	var out Report
	require.NoError(t, yaml.Unmarshal(data, &out))
	assert.Equal(t, in, out)
}
