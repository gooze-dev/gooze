package domain

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	m "gooze.dev/pkg/gooze/internal/model"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// WorkspaceMode selects how a worker builds a mutant without changing the
// project's files.
type WorkspaceMode string

const (
	// WorkspaceOverlay tests each mutant in the project itself: go test reads
	// the mutated file through -overlay. Nothing is copied, and the build cache
	// of the project's real paths is reused. The empty mode means overlay.
	WorkspaceOverlay WorkspaceMode = "overlay"
	// WorkspaceDeepCopy tests each mutant in a per-worker copy of the project, for
	// tests that don't see an overlay: ones that build the project's code in a
	// subprocess or read its source files, or that write next to themselves.
	WorkspaceDeepCopy WorkspaceMode = "deep-copy"
)

// Orchestrator coordinates applying a mutation and running the corresponding
// tests to determine whether the mutation is killed or survives.
type Orchestrator interface {
	// TestMutation runs a single mutation in a throwaway workspace.
	TestMutation(ctx context.Context, mode WorkspaceMode, mutation m.Mutation) (m.Result, error)
	// NewWorkspace returns a reusable workspace. Each worker goroutine should
	// own one, so its temporary files (and, in deep-copy mode, the project copy) are
	// made once per worker instead of once per mutation. A Workspace is NOT safe
	// for concurrent use.
	NewWorkspace(mode WorkspaceMode) Workspace
}

// Workspace tests mutations of a project for one worker, in the way its
// WorkspaceMode selects.
type Workspace interface {
	Run(ctx context.Context, mutation m.Mutation) (m.Result, error)
	Close(ctx context.Context)
}

type orchestrator struct {
	fsAdapter   SourceFSAdapter
	testAdapter TestRunnerAdapter
}

// NewOrchestrator constructs an Orchestrator backed by the provided
// filesystem and test runner adapters.
func NewOrchestrator(fsAdapter SourceFSAdapter, testAdapter TestRunnerAdapter) Orchestrator {
	return &orchestrator{
		fsAdapter:   fsAdapter,
		testAdapter: testAdapter,
	}
}

// NewWorkspace returns a fresh reusable workspace for mode.
func (to *orchestrator) NewWorkspace(mode WorkspaceMode) Workspace {
	if mode == WorkspaceDeepCopy {
		return &workspace{
			fsAdapter:   to.fsAdapter,
			testAdapter: to.testAdapter,
		}
	}

	return &overlayWorkspace{
		fsAdapter:   to.fsAdapter,
		testAdapter: to.testAdapter,
	}
}

// TestMutation runs a single mutation in a throwaway workspace.
func (to *orchestrator) TestMutation(ctx context.Context, mode WorkspaceMode, mutation m.Mutation) (m.Result, error) {
	ws := to.NewWorkspace(mode)
	defer ws.Close(ctx)

	return ws.Run(ctx, mutation)
}

// workspace is a per-worker copy of a project in which mutations are applied
// and tested. It lazily copies the project the first time it sees a mutation
// for a given project root and reuses that copy for subsequent mutations of the
// same project, restoring the mutated file after each run.
type workspace struct {
	fsAdapter   SourceFSAdapter
	testAdapter TestRunnerAdapter

	projectRoot m.Path
	tmpDir      m.Path
}

func (ws *workspace) Run(ctx context.Context, mutation m.Mutation) (m.Result, error) {
	if err := ctx.Err(); err != nil {
		return resultForStatus(mutation, rpt.Timeout), nil
	}

	if err := validateMutation(mutation); err != nil {
		return m.Result{}, err
	}

	if mutation.Source.Test == nil {
		return resultForNoTest(mutation), nil
	}

	if err := ws.ensurePrepared(ctx, m.Path(mutation.Source.Origin.FullPath)); err != nil {
		return m.Result{}, err
	}

	tmpSourcePath, err := ws.tmpPath(ctx, m.Path(mutation.Source.Origin.FullPath))
	if err != nil {
		return m.Result{}, err
	}

	tmpTestPath, err := ws.tmpPath(ctx, m.Path(mutation.Source.Test.FullPath))
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
			slog.Error("Failed to restore original file; discarding workspace copy", "path", path, "error", err)
			ws.cleanup(context.WithoutCancel(ctx))
		}
	}, nil
}

func (ws *workspace) writeFile(ctx context.Context, path m.Path, content []byte) error {
	return writeFile(ctx, ws.fsAdapter, path, content)
}

func writeFile(ctx context.Context, fsAdapter SourceFSAdapter, path m.Path, content []byte) error {
	if err := fsAdapter.WriteFile(ctx, path, content, 0o600); err != nil {
		slog.Error("Failed to write file", "path", path, "error", err)
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

func (ws *workspace) runTests(ctx context.Context, testPath m.Path) rpt.TestStatus {
	return testStatus(ctx, ws.testAdapter, ws.tmpDir, testPath, "")
}

// testStatus runs the tests of testPath's package in workDir, reading the
// files mapped in overlay (if any) instead of their originals, and classifies
// the outcome.
func testStatus(ctx context.Context, testAdapter TestRunnerAdapter, workDir, testPath, overlay m.Path) rpt.TestStatus {
	if err := ctx.Err(); err != nil {
		return rpt.Timeout
	}

	output, testErr := testAdapter.RunGoTest(ctx, string(workDir), string(testPath), string(overlay))
	if testErr != nil {
		if ctx.Err() != nil {
			return rpt.Timeout
		}

		if isBuildFailure(output) {
			slog.Debug("Mutation did not compile", "test", testPath)
			return rpt.Error
		}

		return rpt.Killed
	}

	return rpt.Survived
}

func isBuildFailure(output string) bool {
	return strings.Contains(output, "[build failed]") || strings.Contains(output, "[setup failed]")
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

// overlayWorkspace tests mutations in the project itself. It writes the
// mutated source and an overlay file mapping the original path to it into a
// per-worker temp dir, so go test builds the mutant while no file of the
// project is written or copied.
type overlayWorkspace struct {
	fsAdapter   SourceFSAdapter
	testAdapter TestRunnerAdapter

	tmpDir m.Path
}

// overlay is the JSON file go build -overlay reads: each path in Replace is
// built from the file it maps to.
type overlay struct {
	Replace map[string]string
}

func (ws *overlayWorkspace) Run(ctx context.Context, mutation m.Mutation) (m.Result, error) {
	if result, canceled := canceledResult(ctx, mutation); canceled {
		return result, nil
	}

	if err := validateMutation(mutation); err != nil {
		return m.Result{}, err
	}

	if mutation.Source.Test == nil {
		return resultForNoTest(mutation), nil
	}

	sourcePath := m.Path(mutation.Source.Origin.FullPath)

	root, err := ws.fsAdapter.FindProjectRoot(ctx, sourcePath)
	if err != nil {
		slog.Error("Failed to find project root", "sourcePath", sourcePath, "error", err)
		return m.Result{}, fmt.Errorf("failed to find project root: %w", err)
	}

	overlayPath, err := ws.writeOverlay(ctx, sourcePath, mutation.MutatedCode)
	if err != nil {
		return m.Result{}, err
	}

	// The source lives in the same package as its test and its path is
	// absolute, so it selects the package to test from the project root.
	status := testStatus(ctx, ws.testAdapter, root, sourcePath, overlayPath)

	return resultForStatus(mutation, status), nil
}

// writeOverlay writes mutatedCode and an overlay that maps sourcePath to it
// into the worker's temp dir, and returns the overlay's path.
func (ws *overlayWorkspace) writeOverlay(ctx context.Context, sourcePath m.Path, mutatedCode []byte) (m.Path, error) {
	if ws.tmpDir == "" {
		tmpDir, err := ws.fsAdapter.CreateTempDir(ctx, "gooze-overlay-*")
		if err != nil {
			slog.Error("Failed to create temp dir", "error", err)
			return "", fmt.Errorf("failed to create temp dir: %w", err)
		}

		ws.tmpDir = tmpDir
	}

	mutantPath := ws.fsAdapter.JoinPath(ctx, string(ws.tmpDir), "mutant.go")
	if err := writeFile(ctx, ws.fsAdapter, mutantPath, mutatedCode); err != nil {
		return "", err
	}

	content, err := json.Marshal(overlay{Replace: map[string]string{string(sourcePath): string(mutantPath)}})
	if err != nil {
		return "", fmt.Errorf("failed to encode overlay: %w", err)
	}

	overlayPath := ws.fsAdapter.JoinPath(ctx, string(ws.tmpDir), "overlay.json")
	if err := writeFile(ctx, ws.fsAdapter, overlayPath, content); err != nil {
		return "", err
	}

	return overlayPath, nil
}

// Close removes the worker's temp dir, if any.
func (ws *overlayWorkspace) Close(ctx context.Context) {
	if ws.tmpDir == "" {
		return
	}

	if err := ws.fsAdapter.RemoveAll(context.WithoutCancel(ctx), ws.tmpDir); err != nil {
		slog.Error("Failed to cleanup temp dir", "tmpDir", ws.tmpDir, "error", err)
	}

	ws.tmpDir = ""
}

// canceledResult reports whether ctx is done and, if so, the mutation's
// result: a canceled run is a timeout, not an error.
func canceledResult(ctx context.Context, mutation m.Mutation) (m.Result, bool) {
	if ctx.Err() != nil {
		return resultForStatus(mutation, rpt.Timeout), true
	}

	return nil, false
}

func validateMutation(mutation m.Mutation) error {
	if mutation.Source.Origin == nil {
		return fmt.Errorf("source origin is nil")
	}

	return nil
}

func resultForNoTest(mutation m.Mutation) m.Result {
	return resultForStatus(mutation, rpt.Survived)
}

func resultForStatus(mutation m.Mutation, status rpt.TestStatus) m.Result {
	result := m.Result{}
	result[mutation.Type] = []struct {
		MutationID string
		Status     rpt.TestStatus
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
