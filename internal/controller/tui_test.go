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
		mgen.Arithmetic: []m.MutationOutcome{{MutationID: "hash-10", Status: rpt.Survived}},
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
		mgen.Arithmetic: []m.MutationOutcome{{MutationID: "hash-10", Status: rpt.Killed}},
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
		mgen.Arithmetic: []m.MutationOutcome{{MutationID: "hash-1", Status: rpt.Killed}},
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

// recordingModel hands every message it gets to msgs.
type recordingModel struct{ msgs chan tea.Msg }

func (r recordingModel) Init() tea.Cmd { return nil }
func (r recordingModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	select {
	case r.msgs <- msg:
	default:
	}

	return r, nil
}
func (r recordingModel) View() string { return "" }

// headlessTUI is a TUI whose program runs without a terminal: it reads no
// input.
func headlessTUI(output *bytes.Buffer) *TUI {
	tui := NewTUI(output)
	tui.options = []tea.ProgramOption{tea.WithInput(nil)}

	return tui
}

// sentinelMsg marks the end of what a test sent.
type sentinelMsg struct{}

// startRecording starts a TUI whose program records its messages.
func startRecording(t *testing.T) (*TUI, chan tea.Msg) {
	t.Helper()

	msgs := make(chan tea.Msg, 256)
	tui := headlessTUI(&bytes.Buffer{})

	if err := tui.startWithModel(recordingModel{msgs: msgs}); err != nil {
		t.Fatalf("startWithModel error = %v", err)
	}

	t.Cleanup(func() { tui.Close(context.Background()) })

	return tui, msgs
}

// sentOf returns the messages of type T the program got before the sentinel
// sent now.
func sentOf[T tea.Msg](t *testing.T, tui *TUI, msgs chan tea.Msg) []T {
	t.Helper()

	tui.send(sentinelMsg{})

	var got []T

	for {
		select {
		case msg := <-msgs:
			switch msg := msg.(type) {
			case sentinelMsg:
				return got
			case T:
				got = append(got, msg)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("the program did not get the messages")
		}
	}
}

func canceledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	return ctx
}

func TestTUI_DisplaysSendTheirMessages(t *testing.T) {
	tui, msgs := startRecording(t)
	ctx := context.Background()

	tui.DisplayConcurrencyInfo(ctx, 2, 1, 3)
	if got := sentOf[concurrencyMsg](t, tui, msgs); len(got) != 1 || got[0] != (concurrencyMsg{threads: 2, shardIndex: 1, shards: 3}) {
		t.Fatalf("concurrency messages = %+v", got)
	}

	tui.DisplayUpcomingTestsInfo(ctx, 5)
	if got := sentOf[upcomingMsg](t, tui, msgs); len(got) != 1 || got[0].count != 5 {
		t.Fatalf("upcoming messages = %+v", got)
	}

	tui.DisplayStartingTestInfo(ctx, mutationWithOrigin(), 7)
	if got := sentOf[startMutationMsg](t, tui, msgs); len(got) != 1 || got[0].id != "hash" || got[0].thread != 7 {
		t.Fatalf("start messages = %+v, want the first four characters of the ID on thread 7", got)
	}

	tui.DisplayCompletedTestInfo(ctx, mutationWithOrigin(), completedResult())
	if got := sentOf[completedMutationMsg](t, tui, msgs); len(got) != 1 || got[0].id != "hash" || got[0].status != "killed" {
		t.Fatalf("completed messages = %+v, want the first four characters of the ID, killed", got)
	}

	tui.DisplayMutationScore(ctx, domain.MutationScore{Total: 2, Killed: 1, Survived: 1})
	if got := sentOf[mutationScoreMsg](t, tui, msgs); len(got) != 1 || got[0].score.Killed != 1 {
		t.Fatalf("score messages = %+v", got)
	}

	if err := tui.DisplayEstimation(ctx, domain.Estimation{Total: 4}, nil); err != nil {
		t.Fatalf("DisplayEstimation error = %v", err)
	}

	if got := sentOf[estimationMsg](t, tui, msgs); len(got) != 1 || got[0].total != 4 {
		t.Fatalf("estimation messages = %+v", got)
	}

	tui.DisplayUpdateNotice(ctx, domain.UpdateNotice{Latest: "v9.9.9"})
	if got := sentOf[updateNoticeMsg](t, tui, msgs); len(got) != 1 || got[0].latest != "v9.9.9" {
		t.Fatalf("update notices = %+v", got)
	}
}

func TestTUI_DisplayCompletedTestInfo_StatusWithoutAResultIsUnknown(t *testing.T) {
	tui, msgs := startRecording(t)
	mutation := mutationWithOrigin()

	for name, result := range map[string]m.Result{
		"no result of the mutagen": mResultEmpty(),
		"an empty list of results": {mutation.Type: nil},
	} {
		tui.DisplayCompletedTestInfo(context.Background(), mutation, result)

		if got := sentOf[completedMutationMsg](t, tui, msgs); len(got) != 1 || got[0].status != "unknown" {
			t.Fatalf("%s: completed messages = %+v, want one of unknown status", name, got)
		}
	}
}

func TestTUI_DisplaysSendNothingOnceCanceled(t *testing.T) {
	tui, msgs := startRecording(t)
	ctx := canceledContext()

	tui.DisplayConcurrencyInfo(ctx, 2, 1, 3)
	tui.DisplayUpcomingTestsInfo(ctx, 5)
	tui.DisplayStartingTestInfo(ctx, mutationWithOrigin(), 7)
	tui.DisplayCompletedTestInfo(ctx, mutationWithOrigin(), completedResult())
	tui.DisplayMutationScore(ctx, domain.MutationScore{Total: 2})
	tui.DisplayUpdateNotice(ctx, domain.UpdateNotice{Latest: "v9.9.9"})

	if err := tui.DisplayEstimation(ctx, domain.Estimation{Total: 4}, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("DisplayEstimation error = %v, want context.Canceled", err)
	}

	tui.send(sentinelMsg{})

	for {
		select {
		case msg := <-msgs:
			switch msg.(type) {
			case sentinelMsg:
				return
			case concurrencyMsg, upcomingMsg, startMutationMsg, completedMutationMsg, mutationScoreMsg, updateNoticeMsg, estimationMsg:
				t.Fatalf("got %#v after the context was canceled", msg)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("the program did not get the sentinel")
		}
	}
}

// renderedBy starts the TUI with options, closes it and returns what it drew.
func renderedBy(t *testing.T, options ...StartOption) string {
	t.Helper()

	var buf bytes.Buffer

	tui := headlessTUI(&buf)
	if err := tui.Start(context.Background(), options...); err != nil {
		t.Fatalf("Start error = %v", err)
	}

	if !tui.started {
		t.Fatal("Start did not start the program")
	}

	tui.Close(context.Background())

	return buf.String()
}

func TestTUI_StartPicksTheModelOfTheMode(t *testing.T) {
	if got := renderedBy(t, WithTestMode()); !strings.Contains(got, "Initializing test execution") {
		t.Errorf("test mode drew %q", got)
	}

	for name, options := range map[string][]StartOption{"estimate mode": {WithEstimateMode()}, "no mode": nil} {
		if got := renderedBy(t, options...); !strings.Contains(got, "Loading mutation list") {
			t.Errorf("%s drew %q", name, got)
		}
	}
}

func TestTUI_StartDoesNothingOnceCanceled(t *testing.T) {
	tui := NewTUI(&bytes.Buffer{})

	if err := tui.Start(canceledContext()); !errors.Is(err, context.Canceled) {
		t.Fatalf("Start error = %v, want context.Canceled", err)
	}

	if tui.started || tui.program != nil {
		t.Fatal("Start started the program with a canceled context")
	}
}

func TestTUI_StartsOneProgram(t *testing.T) {
	tui, _ := startRecording(t)
	program := tui.program

	if err := tui.startWithModel(quitModel{}); err != nil {
		t.Fatalf("second startWithModel error = %v", err)
	}

	if !tui.started || tui.program != program {
		t.Fatal("a second start replaced the running program")
	}
}

// isDone reports whether done is closed within the wait.
func isDone(done <-chan struct{}, wait time.Duration) bool {
	select {
	case <-done:
		return true
	case <-time.After(wait):
		return false
	}
}

func TestTUI_CloseQuitsTheProgramOnce(t *testing.T) {
	tui, _ := startRecording(t)

	tui.Close(canceledContext())

	if tui.closed || isDone(tui.done, 50*time.Millisecond) {
		t.Fatal("Close with a canceled context quit the program")
	}

	tui.Close(context.Background())

	if !tui.closed || !isDone(tui.done, 5*time.Second) {
		t.Fatal("Close did not quit the program")
	}
}

func TestTUI_CloseAndWaitWithoutAProgramReturn(t *testing.T) {
	tui := NewTUI(&bytes.Buffer{})
	tui.started = true

	returned := make(chan struct{})

	go func() {
		tui.Close(context.Background())
		tui.Wait(context.Background())
		close(returned)
	}()

	if !isDone(returned, 5*time.Second) {
		t.Fatal("Close and Wait did not return for a TUI without a program")
	}

	if tui.closed {
		t.Fatal("Close marked a TUI without a program as closed")
	}
}

func TestTUI_WaitBlocksUntilTheProgramQuits(t *testing.T) {
	tui, _ := startRecording(t)

	waited := make(chan struct{})

	go func() {
		tui.Wait(context.Background())
		close(waited)
	}()

	if isDone(waited, 50*time.Millisecond) {
		t.Fatal("Wait returned while the program runs")
	}

	tui.Close(context.Background())

	if !isDone(waited, 5*time.Second) {
		t.Fatal("Wait did not return after the program quit")
	}
}

func TestJoinLines_LeavesItsArgumentsAlone(t *testing.T) {
	lines := []string{"", "a", "b"}

	if got := joinLines(lines...); got != "a\nb" {
		t.Fatalf("joinLines = %q", got)
	}

	if lines[0] != "" {
		t.Fatalf("joinLines wrote %q over its first argument", lines[0])
	}
}
