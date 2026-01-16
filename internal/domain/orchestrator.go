package domain

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"

	"github.com/mouse-blink/gooze/internal/adapter"
	m "github.com/mouse-blink/gooze/internal/model"
)

// Orchestrator coordinates applying a mutation to a temporary copy of
// the project and running the corresponding tests to determine whether the
// mutation is killed or survives.
type Orchestrator interface {
	TestMutation(source m.Source, mutation m.Mutation) (m.Report, error)
	TestMutationV2(mutation m.MutationV2) (m.Result, error)
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

func (to *orchestrator) TestMutationV2(mutation m.MutationV2) (m.Result, error) {
	panic("not implemented")
}

// TestMutation applies a mutation to source code and runs tests to check if the mutation is detected.
func (to *orchestrator) TestMutation(source m.Source, mutation m.Mutation) (m.Report, error) {
	report := m.Report{
		MutationID: mutation.ID,
		SourceFile: mutation.SourceFile,
		Killed:     false,
	}

	// If no test file, mutation survives
	if source.Test == "" {
		report.Output = "no test file specified"
		return report, nil
	}

	// Setup temporary testing environment
	tmpDir, projectRoot, err := to.setupMutationTest(source)
	if err != nil {
		report.Error = err
		return report, err
	}
	defer to.cleanupTempDir(tmpDir)

	// Apply mutation
	if err := to.writeMutatedSource(tmpDir, projectRoot, source, mutation); err != nil {
		report.Error = err
		return report, err
	}

	// Run test and evaluate
	if err := to.evaluateMutation(tmpDir, projectRoot, source, &report); err != nil {
		return report, err
	}

	return report, nil
}

// setupMutationTest prepares the temporary testing environment.
func (to *orchestrator) setupMutationTest(source m.Source) (tmpDir, projectRoot m.Path, err error) {
	// Find project root
	projectRoot, err = to.fsAdapter.FindProjectRoot(source.Origin)
	if err != nil {
		return "", "", fmt.Errorf("failed to find project root: %w", err)
	}

	// Create temp directory
	tmpDir, err = to.fsAdapter.CreateTempDir("gooze-mutation-*")
	if err != nil {
		return "", "", fmt.Errorf("failed to create temp dir: %w", err)
	}

	// Copy project to temp
	if err := to.fsAdapter.CopyDir(projectRoot, tmpDir); err != nil {
		to.cleanupTempDir(tmpDir)
		return "", "", fmt.Errorf("failed to copy project: %w", err)
	}

	return tmpDir, projectRoot, nil
}

// cleanupTempDir removes the temporary directory, logging errors if cleanup fails.
func (to *orchestrator) cleanupTempDir(tmpDir m.Path) {
	if err := to.fsAdapter.RemoveAll(tmpDir); err != nil {
		// Log but don't fail on cleanup errors
		_ = err
	}
}

// writeMutatedSource applies mutation and writes to temp directory.
func (to *orchestrator) writeMutatedSource(tmpDir, projectRoot m.Path, source m.Source, mutation m.Mutation) error {
	// Get relative path
	relSourcePath, err := to.fsAdapter.RelPath(projectRoot, source.Origin)
	if err != nil {
		return fmt.Errorf("failed to get relative source path: %w", err)
	}

	tmpSourcePath := to.fsAdapter.JoinPath(string(tmpDir), string(relSourcePath))

	// Read original
	originalContent, err := to.fsAdapter.ReadFile(tmpSourcePath)
	if err != nil {
		return fmt.Errorf("failed to read source file: %w", err)
	}

	// Apply mutation
	mutatedContent, err := applyMutation(originalContent, mutation)
	if err != nil {
		return fmt.Errorf("failed to apply mutation: %w", err)
	}

	// Write mutated
	if err := to.fsAdapter.WriteFile(tmpSourcePath, mutatedContent, 0o600); err != nil {
		return fmt.Errorf("failed to write mutated file: %w", err)
	}

	return nil
}

// evaluateMutation runs tests and determines if mutation was killed.
func (to *orchestrator) evaluateMutation(tmpDir, projectRoot m.Path, source m.Source, report *m.Report) error {
	// Get relative test path
	relTestPath, err := to.fsAdapter.RelPath(projectRoot, source.Test)
	if err != nil {
		return fmt.Errorf("failed to get relative test path: %w", err)
	}

	tmpTestPath := to.fsAdapter.JoinPath(string(tmpDir), string(relTestPath))

	// Run test
	output, testErr := to.testAdapter.RunGoTest(string(tmpDir), string(tmpTestPath))
	report.Output = output

	// If test failed, mutation was killed
	if testErr != nil {
		report.Killed = true
	}

	return nil
}

// applyMutation applies a mutation to source code content.
func applyMutation(content []byte, mutation m.Mutation) ([]byte, error) {
	lines := splitLines(content)
	if mutation.Line < 1 || mutation.Line > len(lines) {
		return nil, fmt.Errorf("line %d out of range (file has %d lines)", mutation.Line, len(lines))
	}

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, "", content, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse content: %w", err)
	}

	// Verify mutation exists
	if !findMutation(fset, file, mutation) {
		return nil, fmt.Errorf("mutation not found at line %d, column %d", mutation.Line, mutation.Column)
	}

	// Apply replacement
	targetLine := lines[mutation.Line-1]
	newLine := replaceInLine(targetLine, mutation)
	lines[mutation.Line-1] = newLine

	return joinLines(lines), nil
}

// findMutation verifies if the mutation exists in the AST.
func findMutation(fset *token.FileSet, file *ast.File, mutation m.Mutation) bool {
	var found bool

	ast.Inspect(file, func(n ast.Node) bool {
		if found {
			return false
		}

		switch mutation.Type {
		case m.MutationArithmetic:
			found = checkArithmeticMutation(n, fset, mutation)
		case m.MutationBoolean:
			found = checkBooleanMutation(n, fset, mutation)
		}

		return !found
	})

	return found
}

// replaceInLine replaces the operator or text at the specified column in a line.
func replaceInLine(line string, mutation m.Mutation) string {
	if mutation.Column < 1 || mutation.Column > len(line) {
		return line
	}

	runes := []rune(line)

	var original, mutated string
	if mutation.Type == m.MutationBoolean {
		original = mutation.OriginalText
		mutated = mutation.MutatedText
	} else {
		original = mutation.OriginalOp.String()
		mutated = mutation.MutatedOp.String()
	}

	col := mutation.Column - 1
	if col+len(original) <= len(runes) && string(runes[col:col+len(original)]) == original {
		return string(runes[:col]) + mutated + string(runes[col+len(original):])
	}

	return line
}

func checkArithmeticMutation(n ast.Node, fset *token.FileSet, mutation m.Mutation) bool {
	binExpr, ok := n.(*ast.BinaryExpr)
	if !ok {
		return false
	}

	pos := fset.Position(binExpr.OpPos)

	return pos.Line == mutation.Line && pos.Column == mutation.Column && binExpr.Op == mutation.OriginalOp
}

func checkBooleanMutation(n ast.Node, fset *token.FileSet, mutation m.Mutation) bool {
	ident, ok := n.(*ast.Ident)
	if !ok {
		return false
	}

	pos := fset.Position(ident.Pos())

	return pos.Line == mutation.Line && pos.Column == mutation.Column && ident.Name == mutation.OriginalText
}

// splitLines splits content into lines, preserving line endings.
func splitLines(content []byte) []string {
	s := string(content)

	var lines []string

	start := 0

	for i := range len(s) {
		if s[i] == '\n' {
			lines = append(lines, s[start:i+1])
			start = i + 1
		}
	}

	if start < len(s) {
		lines = append(lines, s[start:])
	}

	return lines
}

// joinLines joins lines back into content.
func joinLines(lines []string) []byte {
	var result string
	for _, line := range lines {
		result += line
	}

	return []byte(result)
}
