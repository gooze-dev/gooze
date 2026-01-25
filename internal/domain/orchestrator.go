package domain

import (
	"fmt"

	"github.com/mouse-blink/gooze/internal/adapter"
	m "github.com/mouse-blink/gooze/internal/model"
)

// Orchestrator coordinates applying a mutation to a temporary copy of
// the project and running the corresponding tests to determine whether the
// mutation is killed or survives.
type Orchestrator interface {
	TestMutation(mutation m.Mutation) (m.Result, error)
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

func (to *orchestrator) TestMutation(mutation m.Mutation) (m.Result, error) {
	if err := to.validateMutation(mutation); err != nil {
		return m.Result{}, err
	}

	if mutation.Source.Test == nil {
		return to.resultForNoTest(mutation), nil
	}

	projectRoot, tmpDir, err := to.prepareWorkspace(mutation.Source.Origin.FullPath)
	if tmpDir != "" {
		defer to.cleanupTempDir(tmpDir)
	}

	if err != nil {
		return m.Result{}, err
	}

	tmpSourcePath, err := to.buildTempSourcePath(projectRoot, tmpDir, mutation.Source.Origin.FullPath)
	if err != nil {
		return m.Result{}, err
	}

	if err := to.writeMutatedFile(tmpSourcePath, mutation.MutatedCode); err != nil {
		return m.Result{}, err
	}

	tmpTestPath, err := to.buildTempTestPath(projectRoot, tmpDir, mutation.Source.Test.FullPath)
	if err != nil {
		return m.Result{}, err
	}

	status := to.runTests(tmpDir, tmpTestPath)

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

func (to *orchestrator) prepareWorkspace(sourcePath m.Path) (m.Path, m.Path, error) {
	projectRoot, err := to.fsAdapter.FindProjectRoot(sourcePath)
	if err != nil {
		return "", "", fmt.Errorf("failed to find project root: %w", err)
	}

	tmpDir, err := to.fsAdapter.CreateTempDir("gooze-mutation-*")
	if err != nil {
		return "", "", fmt.Errorf("failed to create temp dir: %w", err)
	}

	if err := to.fsAdapter.CopyDir(projectRoot, tmpDir); err != nil {
		return projectRoot, tmpDir, fmt.Errorf("failed to copy project: %w", err)
	}

	return projectRoot, tmpDir, nil
}

func (to *orchestrator) buildTempSourcePath(projectRoot, tmpDir, sourcePath m.Path) (m.Path, error) {
	relSourcePath, err := to.fsAdapter.RelPath(projectRoot, sourcePath)
	if err != nil {
		return "", fmt.Errorf("failed to get relative source path: %w", err)
	}

	return to.fsAdapter.JoinPath(string(tmpDir), string(relSourcePath)), nil
}

func (to *orchestrator) buildTempTestPath(projectRoot, tmpDir, testPath m.Path) (m.Path, error) {
	relTestPath, err := to.fsAdapter.RelPath(projectRoot, testPath)
	if err != nil {
		return "", fmt.Errorf("failed to get relative test path: %w", err)
	}

	return to.fsAdapter.JoinPath(string(tmpDir), string(relTestPath)), nil
}

func (to *orchestrator) writeMutatedFile(path m.Path, content []byte) error {
	if err := to.fsAdapter.WriteFile(path, content, 0o600); err != nil {
		return fmt.Errorf("failed to write mutated file: %w", err)
	}

	return nil
}

func (to *orchestrator) runTests(tmpDir, testPath m.Path) m.TestStatus {
	_, testErr := to.testAdapter.RunGoTest(string(tmpDir), string(testPath))
	if testErr != nil {
		return m.Killed
	}

	return m.Survived
}

// cleanupTempDir removes the temporary directory, logging errors if cleanup fails.
func (to *orchestrator) cleanupTempDir(tmpDir m.Path) {
	if err := to.fsAdapter.RemoveAll(tmpDir); err != nil {
		// Log but don't fail on cleanup errors
		_ = err
	}
}
