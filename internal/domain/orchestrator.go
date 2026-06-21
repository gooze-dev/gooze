package domain

import (
	"context"
	"fmt"
	"log/slog"

	"gooze.dev/pkg/gooze/internal/adapter"
	m "gooze.dev/pkg/gooze/internal/model"
)

// Orchestrator coordinates applying a mutation to a temporary copy of
// the project and running the corresponding tests to determine whether the
// mutation is killed or survives.
type Orchestrator interface {
	// TestMutation runs a single mutation in a throwaway workspace.
	TestMutation(ctx context.Context, mutation m.Mutation) (m.Result, error)
	// NewWorkspace returns a reusable workspace. Each worker goroutine should
	// own one so the project is copied once per worker instead of once per
	// mutation. A Workspace is NOT safe for concurrent use.
	NewWorkspace() Workspace
}

// Workspace is a per-worker copy of a project in which mutations are applied and
// tested. It lazily copies the project the first time it sees a mutation for a
// given project root and reuses that copy for subsequent mutations of the same
// project, restoring the mutated file after each run.
type Workspace interface {
	Run(ctx context.Context, mutation m.Mutation) (m.Result, error)
	Close(ctx context.Context)
}

type orchestrator struct {
	fsAdapter   adapter.SourceFSAdapter
	testAdapter adapter.TestRunnerAdapter
}

// NewOrchestrator constructs an Orchestrator backed by the provided
// filesystem and test runner adapters.
func NewOrchestrator(fsAdapter adapter.SourceFSAdapter, testAdapter adapter.TestRunnerAdapter) Orchestrator {
	return &orchestrator{
		fsAdapter:   fsAdapter,
		testAdapter: testAdapter,
	}
}

// NewWorkspace returns a fresh reusable workspace.
func (to *orchestrator) NewWorkspace() Workspace {
	return &workspace{
		fsAdapter:   to.fsAdapter,
		testAdapter: to.testAdapter,
	}
}

// TestMutation runs a single mutation in a throwaway workspace.
func (to *orchestrator) TestMutation(ctx context.Context, mutation m.Mutation) (m.Result, error) {
	ws := to.NewWorkspace()
	defer ws.Close(ctx)

	return ws.Run(ctx, mutation)
}

type workspace struct {
	fsAdapter   adapter.SourceFSAdapter
	testAdapter adapter.TestRunnerAdapter

	projectRoot m.Path
	tmpDir      m.Path
}

func (ws *workspace) Run(ctx context.Context, mutation m.Mutation) (m.Result, error) {
	if err := ctx.Err(); err != nil {
		return resultForStatus(mutation, m.Timeout), nil
	}

	if err := validateMutation(mutation); err != nil {
		return m.Result{}, err
	}

	if mutation.Source.Test == nil {
		return resultForNoTest(mutation), nil
	}

	if err := ws.ensurePrepared(ctx, mutation.Source.Origin.FullPath); err != nil {
		return m.Result{}, err
	}

	tmpSourcePath, err := ws.tmpPath(ctx, mutation.Source.Origin.FullPath)
	if err != nil {
		return m.Result{}, err
	}

	tmpTestPath, err := ws.tmpPath(ctx, mutation.Source.Test.FullPath)
	if err != nil {
		return m.Result{}, err
	}

	restore, err := ws.applyMutation(ctx, tmpSourcePath, mutation.MutatedCode)
	if err != nil {
		return m.Result{}, err
	}

	defer restore()

	status := ws.runTests(ctx, tmpTestPath)

	return resultForStatus(mutation, status), nil
}

// Close removes the workspace's temporary copy, if any.
func (ws *workspace) Close(ctx context.Context) {
	// Use a cancellation-free context so cleanup still happens even if the run
	// was canceled or timed out.
	ws.cleanup(context.WithoutCancel(ctx))
}

// ensurePrepared makes sure the workspace holds a copy of the project that owns
// sourcePath, reusing the existing copy when the project root is unchanged.
func (ws *workspace) ensurePrepared(ctx context.Context, sourcePath m.Path) error {
	root, err := ws.fsAdapter.FindProjectRoot(ctx, sourcePath)
	if err != nil {
		slog.Error("Failed to find project root", "sourcePath", sourcePath, "error", err)
		return fmt.Errorf("failed to find project root: %w", err)
	}

	if ws.tmpDir != "" && ws.projectRoot == root {
		return nil
	}

	ws.cleanup(ctx)

	tmpDir, err := ws.fsAdapter.CreateTempDir(ctx, "gooze-mutation-*")
	if err != nil {
		slog.Error("Failed to create temp dir", "error", err)
		return fmt.Errorf("failed to create temp dir: %w", err)
	}

	if err := ws.fsAdapter.CopyDir(ctx, root, tmpDir); err != nil {
		slog.Error("Failed to copy project to temp dir", "projectRoot", root, "tmpDir", tmpDir, "error", err)

		if removeErr := ws.fsAdapter.RemoveAll(context.WithoutCancel(ctx), tmpDir); removeErr != nil {
			slog.Error("Failed to clean up temp dir after copy failure", "tmpDir", tmpDir, "error", removeErr)
		}

		return fmt.Errorf("failed to copy project: %w", err)
	}

	ws.projectRoot = root
	ws.tmpDir = tmpDir

	return nil
}

func (ws *workspace) tmpPath(ctx context.Context, fullPath m.Path) (m.Path, error) {
	relPath, err := ws.fsAdapter.RelPath(ctx, ws.projectRoot, fullPath)
	if err != nil {
		slog.Error("Failed to get relative path", "projectRoot", ws.projectRoot, "path", fullPath, "error", err)
		return "", fmt.Errorf("failed to get relative path: %w", err)
	}

	return ws.fsAdapter.JoinPath(ctx, string(ws.tmpDir), string(relPath)), nil
}

// applyMutation writes the mutated source into the workspace and returns a
// function that restores the original content, so the workspace can be reused
// for the next mutation. Restoration uses a cancellation-free context so it runs
// even if the mutation timed out.
func (ws *workspace) applyMutation(ctx context.Context, path m.Path, mutatedCode []byte) (func(), error) {
	original, err := ws.fsAdapter.ReadFile(ctx, path)
	if err != nil {
		slog.Error("Failed to read original file", "path", path, "error", err)
		return nil, fmt.Errorf("failed to read original file: %w", err)
	}

	if err := ws.writeFile(ctx, path, mutatedCode); err != nil {
		return nil, err
	}

	return func() {
		if err := ws.writeFile(context.WithoutCancel(ctx), path, original); err != nil {
			slog.Error("Failed to restore original file", "path", path, "error", err)
		}
	}, nil
}

func (ws *workspace) writeFile(ctx context.Context, path m.Path, content []byte) error {
	if err := ws.fsAdapter.WriteFile(ctx, path, content, 0o600); err != nil {
		slog.Error("Failed to write file", "path", path, "error", err)
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

func (ws *workspace) runTests(ctx context.Context, testPath m.Path) m.TestStatus {
	if err := ctx.Err(); err != nil {
		return m.Timeout
	}

	_, testErr := ws.testAdapter.RunGoTest(ctx, string(ws.tmpDir), string(testPath))
	if testErr != nil {
		if ctx.Err() != nil {
			return m.Timeout
		}

		return m.Killed
	}

	return m.Survived
}

func (ws *workspace) cleanup(ctx context.Context) {
	if ws.tmpDir == "" {
		return
	}

	if err := ws.fsAdapter.RemoveAll(ctx, ws.tmpDir); err != nil {
		slog.Error("Failed to cleanup temp dir", "tmpDir", ws.tmpDir, "error", err)
	}

	ws.tmpDir = ""
	ws.projectRoot = ""
}

func validateMutation(mutation m.Mutation) error {
	if mutation.Source.Origin == nil {
		return fmt.Errorf("source origin is nil")
	}

	return nil
}

func resultForNoTest(mutation m.Mutation) m.Result {
	return resultForStatus(mutation, m.Survived)
}

func resultForStatus(mutation m.Mutation, status m.TestStatus) m.Result {
	result := m.Result{}
	result[mutation.Type] = []struct {
		MutationID string
		Status     m.TestStatus
		Err        error
	}{
		{
			MutationID: mutation.ID,
			Status:     status,
			Err:        nil,
		},
	}

	return result
}
