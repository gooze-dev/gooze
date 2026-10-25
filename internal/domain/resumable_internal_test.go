package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

func TestResumable(t *testing.T) {
	arithmetic := []string{"arithmetic"}

	for _, tc := range []struct {
		scope *rpt.Scope
		want  bool
	}{
		{scope: nil, want: false},
		{scope: &rpt.Scope{}, want: false},
		{scope: &rpt.Scope{Interrupted: true}, want: true},
		{scope: &rpt.Scope{Mutagens: arithmetic}, want: true},
		{scope: &rpt.Scope{Mutagens: arithmetic, Interrupted: true}, want: true},
		{scope: &rpt.Scope{Mutagens: arithmetic, Base: "main"}, want: false},
		{scope: &rpt.Scope{Mutagens: arithmetic, Lines: []rpt.LineRange{{Start: 1, End: 2}}}, want: false},
		{scope: &rpt.Scope{Mutagens: arithmetic, Run: "^Add$"}, want: false},
	} {
		assert.Equal(t, tc.want, resumable(tc.scope), "%+v", tc.scope)
	}
}
