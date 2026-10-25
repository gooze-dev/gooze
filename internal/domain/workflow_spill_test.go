package domain_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domain "gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
	pkg "gooze.dev/pkg/gooze/pkg"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// fullSpill is a report spill whose writes fail once it holds room reports.
type fullSpill struct {
	pkg.FileSpill[m.Report]
	room uint64
}

var errDiskFull = errors.New("no space left on device")

func (s *fullSpill) Append(report m.Report) error {
	if s.Len() == s.room {
		return errDiskFull
	}

	return s.FileSpill.Append(report)
}

// waitingWorkspace kills every mutant. It holds waitFor for half a second, so
// that a stop lands before the next mutant is dispatched.
type waitingWorkspace struct {
	mu      sync.Mutex
	waitFor string
	ran     []string
}

func (ws *waitingWorkspace) Run(_ context.Context, mutation m.Mutation) (m.Result, error) {
	ws.mu.Lock()
	ws.ran = append(ws.ran, mutation.ID)
	ws.mu.Unlock()

	if mutation.ID == ws.waitFor {
		time.Sleep(500 * time.Millisecond)
	}

	return m.Result{mutation.Type: {{MutationID: mutation.ID, Status: rpt.Killed}}}, nil
}

func (ws *waitingWorkspace) Close(context.Context) {}

func TestWorkflow_Test_AFailedReportSpillWriteSavesFinishedMutants(t *testing.T) {
	fx := newInterruptFixture(t)

	var path string

	domain.SetReportSpill(t, func() (pkg.FileSpill[m.Report], error) {
		spill, err := pkg.NewFileSpill[m.Report]()
		require.NoError(t, err)

		path = spill.Path()

		return &fullSpill{FileSpill: spill, room: 2}, nil
	})

	// a-3's report doesn't fit; a-4 may be in flight then and is dropped.
	ws := &waitingWorkspace{waitFor: "a-4"}
	err, score := fx.runWith(context.Background(), t, ws, cachedTestArgs(fx.reports))

	require.ErrorIs(t, err, errDiskFull)
	assert.ErrorContains(t, err, path)
	assert.NotContains(t, ws.ran, "c-1", "the run stops")
	assert.Equal(t, map[string]*rpt.Scope{"a-1": interrupted, "a-2": interrupted}, fx.stored(t))
	assert.True(t, score.Interrupted)
	assert.Equal(t, 2, score.Tested)
	assert.Equal(t, 6, score.Planned)
}
