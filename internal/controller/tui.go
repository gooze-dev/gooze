package controller

import (
	"context"
	"io"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	m "gooze.dev/pkg/gooze/internal/model"
)

// TUI implements UI using Bubble Tea for interactive display.
type TUI struct {
	output  io.Writer
	program *tea.Program
	mu      sync.Mutex
	started bool
	done    chan struct{}
	closed  bool
}

// NewTUI creates a new TUI.
func NewTUI(output io.Writer) *TUI {
	return &TUI{output: output}
}

// Start initializes the UI with the specified mode.
func (t *TUI) Start(ctx context.Context, options ...StartOption) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	config := &StartConfig{mode: ModeEstimate}
	for _, opt := range options {
		opt(config)
	}

	var model tea.Model
	if config.mode == ModeTest {
		model = newTestExecutionModel()
	} else {
		model = newEstimateModel()
	}

	return t.startWithModel(model)
}

// startWithModel initializes the UI with a specific model.
func (t *TUI) startWithModel(model tea.Model) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.started {
		return nil
	}

	// Use alt screen to hide the command line
	t.program = tea.NewProgram(
		model,
		tea.WithOutput(t.output),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	t.done = make(chan struct{})
	t.started = true

	go func() {
		_, _ = t.program.Run()
		close(t.done)
	}()

	return nil
}

// Close finalizes the UI.
func (t *TUI) Close(ctx context.Context) {
	t.mu.Lock()

	if !t.started || t.program == nil || t.closed {
		t.mu.Unlock()
		return
	}

	if err := ctx.Err(); err != nil {
		t.mu.Unlock()
		return
	}

	t.closed = true
	program := t.program
	done := t.done
	t.mu.Unlock()

	program.Send(tea.Quit())
	<-done
}

// Wait blocks until the UI is closed by the user.
func (t *TUI) Wait(ctx context.Context) {
	t.mu.Lock()

	if !t.started || t.program == nil {
		t.mu.Unlock()
		return
	}

	done := t.done
	t.mu.Unlock()

	select {
	case <-done:
	case <-ctx.Done():
	}

	<-done
}

// DisplayEstimation prints the estimation results or error.
func (t *TUI) DisplayEstimation(ctx context.Context, mutations []m.Mutation, err error) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	t.ensureStarted(ctx)

	if err != nil {
		t.send(estimationMsg{err: err})
		return err
	}

	fileStats := make(map[string]fileStat)

	for _, mutation := range mutations {
		if mutation.Source.Origin == nil {
			continue
		}

		fileHash := mutation.Source.Origin.Hash
		if fileHash == "" {
			fileHash = string(mutation.Source.Origin.ShortPath)
		}

		stat := fileStats[fileHash]
		stat.path = string(mutation.Source.Origin.ShortPath)
		stat.count++
		fileStats[fileHash] = stat
	}

	t.send(estimationMsg{
		total:     len(mutations),
		paths:     len(fileStats),
		fileStats: fileStats,
	})

	// Don't close immediately - let user interact with the UI
	// User will press 'q' to quit
	return nil
}

// DisplayConcurrencyInfo shows concurrency settings.
func (t *TUI) DisplayConcurrencyInfo(ctx context.Context, threads int, shardIndex int, count int) {
	if err := ctx.Err(); err != nil {
		return
	}

	t.ensureStarted(ctx)
	t.send(concurrencyMsg{threads: threads, shardIndex: shardIndex, shards: count})
}

// DisplayUpcomingTestsInfo shows the number of upcoming mutations to be tested.
func (t *TUI) DisplayUpcomingTestsInfo(ctx context.Context, i int) {
	if err := ctx.Err(); err != nil {
		return
	}

	t.ensureStarted(ctx)
	t.send(upcomingMsg{count: i})
}

// DisplayStartingTestInfo shows info about the mutation test starting.
func (t *TUI) DisplayStartingTestInfo(ctx context.Context, currentMutation m.Mutation, threadID int) {
	if err := ctx.Err(); err != nil {
		return
	}

	t.ensureStarted(ctx)

	path := ""
	fileHash := ""

	if currentMutation.Source.Origin != nil {
		path = string(currentMutation.Source.Origin.ShortPath)
		fileHash = currentMutation.Source.Origin.Hash
	}

	t.send(startMutationMsg{
		id:          currentMutation.ID[:4],
		thread:      threadID,
		kind:        currentMutation.Type.Name,
		fileHash:    fileHash,
		displayPath: path,
	})
}

// DisplayCompletedTestInfo shows info about the completed mutation test.
func (t *TUI) DisplayCompletedTestInfo(ctx context.Context, currentMutation m.Mutation, mutationResult m.Result) {
	if err := ctx.Err(); err != nil {
		return
	}

	t.ensureStarted(ctx)

	status := "unknown"
	if results, ok := mutationResult[currentMutation.Type]; ok && len(results) > 0 {
		status = formatTestStatus(results[0].Status)
	}

	path := ""
	fileHash := ""
	diff := []byte(nil)

	if currentMutation.Source.Origin != nil {
		path = string(currentMutation.Source.Origin.ShortPath)
		fileHash = currentMutation.Source.Origin.Hash
	}

	if status != formatTestStatus(m.Killed) && len(currentMutation.DiffCode) > 0 {
		diff = currentMutation.DiffCode
	}

	t.send(completedMutationMsg{
		id:          currentMutation.ID[:4],
		kind:        currentMutation.Type.Name,
		fileHash:    fileHash,
		displayPath: path,
		status:      status,
		diff:        diff,
	})
}

// DisplayMutationScore shows the final mutation score.
func (t *TUI) DisplayMutationScore(ctx context.Context, score float64) {
	if err := ctx.Err(); err != nil {
		return
	}

	t.ensureStarted(ctx)
	t.send(mutationScoreMsg{score: score})
}

func (t *TUI) ensureStarted(ctx context.Context) {
	_ = t.Start(ctx)
}

func (t *TUI) send(msg tea.Msg) {
	t.mu.Lock()
	program := t.program
	started := t.started
	t.mu.Unlock()

	if !started || program == nil {
		return
	}

	program.Send(msg)
}
