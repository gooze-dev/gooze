package adapter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	m "gooze.dev/pkg/gooze/internal/model"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

func TestLocalReportStore_CheckUpdates_OnlyLinesOrFunctionsLeftOutMakeASourcePartial(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		scope   *rpt.Scope
		partial bool
	}{
		"full":        {nil, false},
		"mutagens":    {&rpt.Scope{Mutagens: []string{"arithmetic"}}, false},
		"lines":       {&rpt.Scope{Lines: []rpt.LineRange{{Start: 3, End: 3}}}, true},
		"base":        {&rpt.Scope{Base: "abc"}, true},
		"run":         {&rpt.Scope{Run: "^Add$", Mutagens: []string{"arithmetic"}}, true},
		"interrupted": {&rpt.Scope{Interrupted: true}, true},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			rs := &LocalReportStore{}
			source := rpt.Source{Origin: &rpt.File{FullPath: "/abs/a.go", Hash: "same"}}

			require.NoError(t, rs.SaveReports(context.Background(), m.Path(dir), reportSpill(t,
				m.Report{Source: source, Result: m.Result{mgen.Arithmetic: {{MutationID: "a1", Status: rpt.Killed}}}, Scope: tc.scope},
			)))

			changed, deleted, err := rs.CheckUpdates(context.Background(), m.Path(dir), sourceSpill(t, source), nil)
			require.NoError(t, err)
			_ = spillItems(t, deleted)

			assert.Equal(t, tc.partial, len(spillItems(t, changed)) == 1)
		})
	}
}
