package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"

	rpt "gooze.dev/pkg/gooze/pkg/report"
)

func TestSameSelection(t *testing.T) {
	base := rpt.Selection{Diff: "b", Base: "b", Run: "b", Mutagens: []string{"b"}}
	assert.True(t, sameSelection(base, rpt.Selection{Diff: "b", Base: "b", Run: "b", Mutagens: []string{"b"}}))

	for _, other := range []string{"a", "c"} {
		for name, changed := range map[string]rpt.Selection{
			"diff":     {Diff: other, Base: "b", Run: "b", Mutagens: []string{"b"}},
			"base":     {Diff: "b", Base: other, Run: "b", Mutagens: []string{"b"}},
			"run":      {Diff: "b", Base: "b", Run: other, Mutagens: []string{"b"}},
			"mutagens": {Diff: "b", Base: "b", Run: "b", Mutagens: []string{other}},
		} {
			assert.False(t, sameSelection(base, changed), "%s %s", name, other)
		}
	}
}
