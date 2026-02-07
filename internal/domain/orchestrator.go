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
	TestMutation(ctx context.Context, mutation m.Mutation) (m.Result, error)
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

func (to *orchestrator) TestMutation(ctx context.Context, mutation m.Mutation) (m.Result, error) {
	if err := ctx.Err(); err != nil {
		return to.resultForStatus(mutation, m.Timeout), nil
	}

	if err := to.validateMutation(mutation); err != nil {
		return m.Result{}, err
	}

	if mutation.Source.Test == nil {
		return to.resultForNoTest(mutation), nil
	}

	projectRoot, tmpDir, err := to.prepareWorkspace(ctx, mutation.Source.Origin.FullPath)
	if tmpDir != "" {
		defer to.cleanupTempDir(ctx, tmpDir)
	}

	if err != nil {
		return m.Result{}, err
	}

	tmpSourcePath, err := to.buildTempSourcePath(ctx, projectRoot, tmpDir, mutation.Source.Origin.FullPath)
	if err != nil {
		return m.Result{}, err
	}

	if err := to.writeMutatedFile(ctx, tmpSourcePath, mutation.MutatedCode); err != nil {
		return m.Result{}, err
	}

	tmpTestPath, err := to.buildTempTestPath(ctx, projectRoot, tmpDir, mutation.Source.Test.FullPath)
	if err != nil {
		return m.Result{}, err
	}

	status := to.runTests(ctx, tmpDir, tmpTestPath)

	return to.resultForStatus(mutation, status), nil
}

func (to *orchestrator) validateMutation(mutation m.Mutation) error {
	if mutation.Source.Origin == nil {
		return fmt.Errorf("source origin is nil")
	}

	return nil
}

func (to *orchestrator) resultForNoTest(mutation m.Mutation) m.Result {
	return to.resultForStatus(mutation, m.Survived)
}

func (to *orchestrator) resultForStatus(mutation m.Mutation, status m.TestStatus) m.Result {
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

func (to *orchestrator) prepareWorkspace(ctx context.Context, sourcePath m.Path) (m.Path, m.Path, error) {
	projectRoot, err := to.fsAdapter.FindProjectRoot(ctx, sourcePath)
	if err != nil {
		slog.Error("Failed to find project root", "sourcePath", sourcePath, "error", err)
		return "", "", fmt.Errorf("failed to find project root: %w", err)
	}

	tmpDir, err := to.fsAdapter.CreateTempDir(ctx, "gooze-mutation-*")
	if err != nil {
		slog.Error("Failed to create temp dir", "error", err)
		return "", "", fmt.Errorf("failed to create temp dir: %w", err)
	}

	if err := to.fsAdapter.CopyDir(ctx, projectRoot, tmpDir); err != nil {
		slog.Error("Failed to copy project to temp dir", "projectRoot", projectRoot, "tmpDir", tmpDir, "error", err)
		return projectRoot, tmpDir, fmt.Errorf("failed to copy project: %w", err)
	}

	return projectRoot, tmpDir, nil
}

func (to *orchestrator) buildTempSourcePath(ctx context.Context, projectRoot, tmpDir, sourcePath m.Path) (m.Path, error) {
	relSourcePath, err := to.fsAdapter.RelPath(ctx, projectRoot, sourcePath)
	if err != nil {
		slog.Error("Failed to get relative source path", "projectRoot", projectRoot, "sourcePath", sourcePath, "error", err)
		return "", fmt.Errorf("failed to get relative source path: %w", err)
	}

	return to.fsAdapter.JoinPath(ctx, string(tmpDir), string(relSourcePath)), nil
}

func (to *orchestrator) buildTempTestPath(ctx context.Context, projectRoot, tmpDir, testPath m.Path) (m.Path, error) {
	relTestPath, err := to.fsAdapter.RelPath(ctx, projectRoot, testPath)
	if err != nil {
		slog.Error("Failed to get relative test path", "projectRoot", projectRoot, "testPath", testPath, "error", err)
		return "", fmt.Errorf("failed to get relative test path: %w", err)
	}

	return to.fsAdapter.JoinPath(ctx, string(tmpDir), string(relTestPath)), nil
}

func (to *orchestrator) writeMutatedFile(ctx context.Context, path m.Path, content []byte) error {
	if err := to.fsAdapter.WriteFile(ctx, path, content, 0o600); err != nil {
		slog.Error("Failed to write mutated file", "path", path, "error", err)
		return fmt.Errorf("failed to write mutated file: %w", err)
	}

	return nil
}

func (to *orchestrator) runTests(ctx context.Context, tmpDir, testPath m.Path) m.TestStatus {
	if err := ctx.Err(); err != nil {
		return m.Timeout
	}

	_, testErr := to.testAdapter.RunGoTest(ctx, string(tmpDir), string(testPath))
	if testErr != nil {
		if ctx.Err() != nil {
			return m.Timeout
		}

		return m.Killed
	}

	return m.Survived
}

// cleanupTempDir removes the temporary directory, logging errors if cleanup fails.
func (to *orchestrator) cleanupTempDir(ctx context.Context, tmpDir m.Path) {
	if err := to.fsAdapter.RemoveAll(ctx, tmpDir); err != nil {
		slog.Error("Failed to cleanup temp dir", "tmpDir", tmpDir, "error", err)
	}
}
