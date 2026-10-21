package controller

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

type quitModel struct{}

func (m quitModel) Init() tea.Cmd { return tea.Quit }
func (m quitModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return m, tea.Quit
}
func (m quitModel) View() string { return "" }

func TestTUI_StartWithModel_WaitAndClose(t *testing.T) {
	var buf bytes.Buffer
	tui := NewTUI(&buf)

	if err := tui.startWithModel(quitModel{}); err != nil {
		t.Fatalf("startWithModel error = %v", err)
	}

	// send while running should go through program.Send
	tui.send(upcomingMsg{count: 2})

	waitDone := make(chan struct{})
	go func() {
		tui.Wait(context.Background())
		close(waitDone)
	}()

	select {
	case <-waitDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Wait() timed out")
	}

	closeDone := make(chan struct{})
	go func() {
		tui.Close(context.Background())
		close(closeDone)
	}()

	select {
	case <-closeDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Close() timed out")
	}
}

func TestTUI_Send_And_EnsureStarted_NoPanic(t *testing.T) {
	var buf bytes.Buffer
	tui := NewTUI(&buf)

	// send before start should be no-op
	tui.send(upcomingMsg{count: 1})

	// ensureStarted should not re-start when already started
	tui.started = true
	tui.ensureStarted(context.Background())
}

func TestTUI_DisplayCompletedTestInfo_WithDiff(t *testing.T) {
	var buf bytes.Buffer
	tui := NewTUI(&buf)

	diffCode := []byte("--- original\n+++ mutated\n@@ -1,1 +1,1 @@\n-\treturn 3 + 5\n+\treturn 3 - 5\n")

	mutation := m.Mutation{
		ID:       "hash-10",
		Type:     mgen.Arithmetic,
		Source:   rpt.Source{Origin: &rpt.File{ShortPath: "test.go", Hash: "hash1"}},
		DiffCode: diffCode,
	}

	survivedResult := m.Result{
		mgen.Arithmetic: []struct {
			MutationID string
			Status     rpt.TestStatus
			Err        error
		}{{MutationID: "hash-10", Status: rpt.Survived}},
	}

	// Start TUI in test mode
	ctx := context.Background()
	if err := tui.Start(ctx, WithTestMode()); err != nil {
		t.Fatalf("Start error = %v", err)
	}

	// Test that DisplayCompletedTestInfo sends message with diff for survived mutation
	tui.DisplayCompletedTestInfo(ctx, mutation, survivedResult)

	// Verify it doesn't panic and the TUI is still functional
	tui.Close(ctx)
}

func TestTUI_DisplayCompletedTestInfo_WithoutDiff(t *testing.T) {
	var buf bytes.Buffer
	tui := NewTUI(&buf)

	mutation := m.Mutation{
		ID:     "hash-10",
		Type:   mgen.Arithmetic,
		Source: rpt.Source{Origin: &rpt.File{ShortPath: "test.go", Hash: "hash1"}},
		// No DiffCode
	}

	killedResult := m.Result{
		mgen.Arithmetic: []struct {
			MutationID string
			Status     rpt.TestStatus
			Err        error
		}{{MutationID: "hash-10", Status: rpt.Killed}},
	}

	// Start TUI in test mode
	ctx := context.Background()
	if err := tui.Start(ctx, WithTestMode()); err != nil {
		t.Fatalf("Start error = %v", err)
	}

	// Test that DisplayCompletedTestInfo sends message without diff for killed mutation
	tui.DisplayCompletedTestInfo(ctx, mutation, killedResult)

	// Verify it doesn't panic and the TUI is still functional
	tui.Close(ctx)
}

func TestTUI_StartWithMouseCellMotion(t *testing.T) {
	var buf bytes.Buffer
	tui := NewTUI(&buf)

	ctx := context.Background()
	// Test that TUI starts with mouse cell motion enabled (should not error)
	if err := tui.Start(ctx); err != nil {
		t.Fatalf("Start error = %v", err)
	}

	tui.Close(ctx)
}

func TestTUI_MultipleClose(t *testing.T) {
	var buf bytes.Buffer
	tui := NewTUI(&buf)

	ctx := context.Background()
	tui.Close(ctx)
	tui.Close(ctx) // Close again should be safe

	tui2 := NewTUI(&buf)
	tui2.Wait(ctx) // Wait without start should be no-op

	tui3 := NewTUI(&buf)
	tui3.Close(ctx) // Close without start should be no-op
}

func TestTUI_DisplayMethods_NoProgram(t *testing.T) {
	var buf bytes.Buffer
	tui := NewTUI(&buf)

	// Avoid starting Bubble Tea program in tests
	tui.started = true

	ctx := context.Background()
	if err := tui.DisplayEstimation(ctx, domain.Estimation{}, nil); err != nil {
		t.Fatalf("DisplayEstimation unexpected error = %v", err)
	}

	if err := tui.DisplayEstimation(ctx, domain.Estimation{}, errSentinel); err == nil {
		t.Fatalf("DisplayEstimation expected error")
	}

	estimation := domain.Estimation{
		Total: 1,
		Files: []domain.FileEstimate{{Path: "a.go", Count: 1}},
	}
	if err := tui.DisplayEstimation(ctx, estimation, nil); err != nil {
		t.Fatalf("DisplayEstimation with mutations error = %v", err)
	}

	tui.DisplayConcurrencyInfo(ctx, 2, 1, 3)
	tui.DisplayUpcomingTestsInfo(ctx, 5)
	tui.DisplayStartingTestInfo(ctx, mutationWithOrigin(), 7)
	tui.DisplayStartingTestInfo(ctx, mutationWithoutOrigin(), 8)
	tui.DisplayCompletedTestInfo(ctx, mutationWithOrigin(), completedResult())
	tui.DisplayCompletedTestInfo(ctx, mutationWithoutOrigin(), mResultEmpty())
	tui.DisplayMutationScore(ctx, domain.MutationScore{Total: 2, Killed: 1, Survived: 1})
}

var errSentinel = errors.New("boom")

func mutationWithOrigin() m.Mutation {
	return m.Mutation{ID: "hash-1", Type: mgen.Arithmetic, Source: rpt.Source{Origin: &rpt.File{ShortPath: "a.go", FullPath: "path/a.go", Hash: "hash-a"}}}
}

func mutationWithoutOrigin() m.Mutation {
	return m.Mutation{ID: "hash-2", Type: mgen.Boolean}
}

func completedResult() m.Result {
	return m.Result{
		mgen.Arithmetic: []struct {
			MutationID string
			Status     rpt.TestStatus
			Err        error
		}{{MutationID: "hash-1", Status: rpt.Killed}},
	}
}

func mResultEmpty() m.Result {
	return m.Result{}
}

func TestWithUpdateNotice(t *testing.T) {
	if got := withUpdateNotice("title", ""); got != "title" {
		t.Fatalf("withUpdateNotice without a release = %q, want title unchanged", got)
	}

	got := withUpdateNotice("title", "v1.3.0")
	if !strings.Contains(got, "title") || !strings.Contains(got, "gooze v1.3.0 available") {
		t.Fatalf("withUpdateNotice = %q, want title plus notice", got)
	}
}

func TestModels_UpdateNoticeMsg(t *testing.T) {
	est, _ := newEstimateModel().Update(updateNoticeMsg{latest: "v1.3.0"})
	if got := est.(estimateModel).updateLatest; got != "v1.3.0" {
		t.Fatalf("estimateModel.updateLatest = %q", got)
	}

	exec, _ := newTestExecutionModel().Update(updateNoticeMsg{latest: "v1.3.0"})
	if got := exec.(testExecutionModel).updateLatest; got != "v1.3.0" {
		t.Fatalf("testExecutionModel.updateLatest = %q", got)
	}
}

func TestTUI_DisplayUpdateNotice_BeforeStart(t *testing.T) {
	tui := NewTUI(&bytes.Buffer{})
	tui.DisplayUpdateNotice(context.Background(), domain.UpdateNotice{Current: "v1.2.0", Latest: "v1.3.0"})

	if tui.notice == nil || tui.notice.Latest != "v1.3.0" {
		t.Fatalf("notice not kept for start: %+v", tui.notice)
	}

	var _ UpdateNotifier = tui
}

func TestJoinLines(t *testing.T) {
	cases := map[string][]string{
		"":     {"", ""},
		"a":    {"a", ""},
		"b":    {"", "b"},
		"a\nb": {"a", "b"},
	}

	for want, lines := range cases {
		if got := joinLines(lines...); got != want {
			t.Errorf("joinLines(%q) = %q, want %q", lines, got, want)
		}
	}
}
