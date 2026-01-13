package domain

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	m "github.com/mouse-blink/gooze/internal/model"
)

const goFileExt = ".go"

// Workflow defines the interface for mutation testing operations.
type Workflow interface {
	GetSources(roots ...m.Path) ([]m.Source, error)
	GenerateMutations(sources m.Source, mutationType m.MutationType) ([]m.Mutation, error)
	EstimateMutations(sources m.Source, mutationType m.MutationType) (int, error)
	TestMutation(sources m.Source, mutation m.Mutation) (m.Report, error)
}

type workflow struct{}

// NewWorkflow creates a new Workflow instance.
func NewWorkflow() Workflow {
	return &workflow{}
}

// GetSources walks directory trees and identifies code scopes for mutation testing.
// Supports multiple paths and ./... suffix for recursive scanning.
// It distinguishes between:
// - Global scope (const, var, type declarations) - for mutations like boolean literals, numbers
// - Init functions - for all mutation types
// - Regular functions - for function-specific mutations.
func (w *workflow) GetSources(roots ...m.Path) ([]m.Source, error) {
	if len(roots) == 0 {
		return []m.Source{}, nil
	}

	seen := make(map[string]struct{})

	var allSources []m.Source

	for _, root := range roots {
		sources, err := w.scanPath(root)
		if err != nil {
			return nil, err
		}

		for _, source := range sources {
			absPath, err := filepath.Abs(string(source.Origin))
			if err != nil {
				absPath = filepath.Clean(string(source.Origin))
			}

			if _, exists := seen[absPath]; !exists {
				seen[absPath] = struct{}{}

				allSources = append(allSources, source)
			}
		}
	}

	return allSources, nil
}

// scanPath scans a single path (with optional /... suffix) for Go source files.
func (w *workflow) scanPath(root m.Path) ([]m.Source, error) {
	rootStr, recursive := parseRootPath(string(root))

	if _, err := os.Stat(rootStr); err != nil {
		return nil, fmt.Errorf("root path error: %w", err)
	}

	var sources []m.Source

	fset := token.NewFileSet()

	err := filepath.Walk(rootStr, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return handleDirectory(path, rootStr, recursive)
		}

		source, shouldInclude, processErr := w.processFile(path, fset)
		if processErr != nil {
			return processErr
		}

		if shouldInclude {
			sources = append(sources, source)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return sources, nil
}

// parseRootPath extracts the root path and recursive flag from a path string.
func parseRootPath(rootStr string) (path string, recursive bool) {
	if len(rootStr) >= 4 && rootStr[len(rootStr)-4:] == "/..." {
		return rootStr[:len(rootStr)-4], true
	}

	return rootStr, false
}

// handleDirectory determines if directory traversal should continue.
func handleDirectory(path, rootStr string, recursive bool) error {
	if !recursive && path != rootStr {
		return filepath.SkipDir
	}

	return nil
}

// processFile parses and extracts scopes from a single Go file.
func (w *workflow) processFile(path string, fset *token.FileSet) (m.Source, bool, error) {
	// Skip non-Go files
	if filepath.Ext(path) != goFileExt {
		return m.Source{}, false, nil
	}

	// Skip test files (e.g., *_test.go)
	if isTestFile(path) {
		return m.Source{}, false, nil
	}

	file, parseErr := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if parseErr != nil {
		// Skip files with parse errors instead of failing the entire scan
		return m.Source{}, false, nil //nolint:nilerr // Intentionally skip parse errors
	}

	scopes := extractScopes(fset, file)
	if len(scopes) == 0 {
		return m.Source{}, false, nil
	}

	functionLines := extractFunctionLines(scopes)
	hasGlobals := hasGlobalScopes(scopes)

	// Only include if has functions/init or global declarations
	if len(functionLines) == 0 && !hasGlobals {
		return m.Source{}, false, nil
	}

	hash, hashErr := hashFile(path)
	if hashErr != nil {
		return m.Source{}, false, fmt.Errorf("hash error for %s: %w", path, hashErr)
	}

	// Automatically detect corresponding test file
	testFile := findTestFile(path)

	source := m.Source{
		Hash:   hash,
		Origin: m.Path(path),
		Test:   testFile,
		Lines:  functionLines,
		Scopes: scopes,
	}

	return source, true, nil
}

// isTestFile returns true if the given path is a Go test file.
func isTestFile(p string) bool {
	if filepath.Ext(p) != goFileExt {
		return false
	}

	return strings.HasSuffix(p, "_test.go")
}

// extractScopes analyzes an AST and returns all relevant code scopes.
func extractScopes(fset *token.FileSet, file *ast.File) []m.CodeScope {
	var scopes []m.CodeScope

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			// Handle package-level declarations (const, var, type)
			if d.Tok == token.CONST || d.Tok == token.VAR {
				for _, spec := range d.Specs {
					if vs, ok := spec.(*ast.ValueSpec); ok {
						for _, name := range vs.Names {
							scope := m.CodeScope{
								Type:      m.ScopeGlobal,
								StartLine: fset.Position(vs.Pos()).Line,
								EndLine:   fset.Position(vs.End()).Line,
								Name:      name.Name,
							}
							scopes = append(scopes, scope)
						}
					}
				}
			}

		case *ast.FuncDecl:
			// Handle functions
			startLine := fset.Position(d.Pos()).Line
			endLine := fset.Position(d.End()).Line

			// Distinguish init functions from regular functions
			scopeType := m.ScopeFunction

			funcName := d.Name.Name
			if funcName == "init" {
				scopeType = m.ScopeInit
			}

			scope := m.CodeScope{
				Type:      scopeType,
				StartLine: startLine,
				EndLine:   endLine,
				Name:      funcName,
			}
			scopes = append(scopes, scope)
		}
	}

	return scopes
}

// extractFunctionLines extracts line numbers for functions only (backward compatibility).
func extractFunctionLines(scopes []m.CodeScope) []int {
	var lines []int

	seen := make(map[int]struct{})

	for _, scope := range scopes {
		// Only include function and init scopes, not global
		if scope.Type == m.ScopeFunction || scope.Type == m.ScopeInit {
			if _, exists := seen[scope.StartLine]; !exists {
				lines = append(lines, scope.StartLine)
				seen[scope.StartLine] = struct{}{}
			}
		}
	}

	return lines
}

// hasGlobalScopes checks if there are any global const/var scopes.
func hasGlobalScopes(scopes []m.CodeScope) bool {
	for _, scope := range scopes {
		if scope.Type == m.ScopeGlobal {
			return true
		}
	}

	return false
}

// hashFile computes SHA-256 hash of a file.
func hashFile(path string) (string, error) {
	f, err := os.Open(path) // #nosec G304 -- path comes from trusted file system walk
	if err != nil {
		return "", err
	}

	defer func() {
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// findTestFile attempts to locate the corresponding test file for a source file.
// For a file like "calc.go", it looks for "calc_test.go" in the same directory.
func findTestFile(sourcePath string) m.Path {
	// Skip if already a test file
	if filepath.Ext(sourcePath) != ".go" {
		return ""
	}

	if len(sourcePath) >= 8 && sourcePath[len(sourcePath)-8:] == "_test.go" {
		return ""
	}

	// Build test file path: source.go -> source_test.go
	dir := filepath.Dir(sourcePath)
	base := filepath.Base(sourcePath)
	base = base[:len(base)-3] // Remove ".go"
	testFile := filepath.Join(dir, base+"_test.go")

	// Check if test file exists
	if _, err := os.Stat(testFile); err == nil {
		return m.Path(testFile)
	}

	return ""
}

// EstimateMutations calculates the total number of mutations for a source and mutation type.
func (w *workflow) EstimateMutations(source m.Source, mutationType m.MutationType) (int, error) {
	if mutationType != m.MutationArithmetic {
		return 0, fmt.Errorf("unsupported mutation type: %v", mutationType)
	}

	mutations, err := w.GenerateMutations(source, mutationType)
	if err != nil {
		return 0, fmt.Errorf("failed to estimate mutations for %s: %w", source.Origin, err)
	}

	return len(mutations), nil
}

// TestMutation applies a mutation to source code and runs tests to check if the mutation is detected.
func (w *workflow) TestMutation(source m.Source, mutation m.Mutation) (m.Report, error) {
	report := m.Report{
		MutationID: mutation.ID,
		SourceFile: mutation.SourceFile,
		Killed:     false,
	}

	// If no test file specified, mutation survives
	if source.Test == "" {
		report.Output = "no test file specified"
		return report, nil
	}

	// Setup temporary testing environment
	tmpDir, projectRoot, err := setupMutationTest(source)
	if err != nil {
		report.Error = err
		return report, err
	}

	defer cleanupTempDir(tmpDir)

	// Apply mutation to source file in temp directory
	if err := writeMutatedSource(tmpDir, projectRoot, source, mutation); err != nil {
		report.Error = err
		return report, err
	}

	// Run test and evaluate mutation
	if err := evaluateMutation(tmpDir, projectRoot, source, &report); err != nil {
		return report, err
	}

	return report, nil
}

// setupMutationTest prepares the temporary testing environment.
func setupMutationTest(source m.Source) (tmpDir, projectRoot string, err error) {
	// Find the project root directory based on go.mod
	projectRoot, err = findProjectRoot(string(source.Origin))
	if err != nil {
		return "", "", fmt.Errorf("failed to find project root: %w", err)
	}

	// Create temporary directory for mutation testing
	tmpDir, err = os.MkdirTemp("", "gooze-mutation-*")
	if err != nil {
		return "", "", fmt.Errorf("failed to create temp dir: %w", err)
	}

	// Copy entire project to temporary directory
	if err := copyDir(projectRoot, tmpDir); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", "", fmt.Errorf("failed to copy project: %w", err)
	}

	return tmpDir, projectRoot, nil
}

// cleanupTempDir removes the temporary directory, logging errors if cleanup fails.
func cleanupTempDir(tmpDir string) {
	if err := os.RemoveAll(tmpDir); err != nil {
		// Log but don't fail on cleanup errors
		_ = err
	}
}

// writeMutatedSource applies mutation and writes to temp directory.
func writeMutatedSource(tmpDir, projectRoot string, source m.Source, mutation m.Mutation) error {
	// Calculate relative paths from project root
	relSourcePath, err := filepath.Rel(projectRoot, string(source.Origin))
	if err != nil {
		return fmt.Errorf("failed to get relative source path: %w", err)
	}

	// Path to source file in temp directory
	tmpSourcePath := filepath.Join(tmpDir, relSourcePath)

	// Read original source file from temp directory
	// #nosec G304 - tmpSourcePath is internally generated, not user input
	originalContent, err := os.ReadFile(tmpSourcePath)
	if err != nil {
		return fmt.Errorf("failed to read source file: %w", err)
	}

	// Apply mutation to create mutated content
	mutatedContent, err := applyMutation(originalContent, mutation)
	if err != nil {
		return fmt.Errorf("failed to apply mutation: %w", err)
	}

	// Write mutated content to temp directory with restricted permissions
	if err := os.WriteFile(tmpSourcePath, mutatedContent, 0o600); err != nil {
		return fmt.Errorf("failed to write mutated file: %w", err)
	}

	return nil
}

// evaluateMutation runs tests and determines if mutation was killed.
func evaluateMutation(tmpDir, projectRoot string, source m.Source, report *m.Report) error {
	// Calculate relative test file path
	relTestPath, err := filepath.Rel(projectRoot, string(source.Test))
	if err != nil {
		return fmt.Errorf("failed to get relative test path: %w", err)
	}

	// Path to test file in temp directory
	tmpTestPath := filepath.Join(tmpDir, relTestPath)

	// Run only the specific test file in temporary directory
	output, testErr := runGoTest(tmpDir, tmpTestPath)
	report.Output = output

	// If test failed (non-zero exit), mutation was killed
	if testErr != nil {
		report.Killed = true
	}

	return nil
}

// applyMutation applies a mutation to source code content.
func applyMutation(content []byte, mutation m.Mutation) ([]byte, error) {
	// First check if the line exists
	lines := splitLines(content)
	if mutation.Line < 1 || mutation.Line > len(lines) {
		return nil, fmt.Errorf("line %d out of range (file has %d lines)", mutation.Line, len(lines))
	}

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, "", content, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse content: %w", err)
	}

	// Find the target mutation position to verify it exists
	var mutationFound bool

	ast.Inspect(file, func(n ast.Node) bool {
		binExpr, ok := n.(*ast.BinaryExpr)
		if !ok {
			return true
		}

		pos := fset.Position(binExpr.OpPos)

		// Check if this is the mutation we want to apply
		if pos.Line == mutation.Line && pos.Column == mutation.Column && binExpr.Op == mutation.OriginalOp {
			mutationFound = true
			return false
		}

		return true
	})

	if !mutationFound {
		return nil, fmt.Errorf("mutation not found at line %d, column %d", mutation.Line, mutation.Column)
	}

	// Replace the operator in the target line
	targetLine := lines[mutation.Line-1]
	newLine := replaceOperatorInLine(targetLine, mutation)
	lines[mutation.Line-1] = newLine

	buf := joinLines(lines)

	return buf, nil
}

// replaceOperatorInLine replaces the operator at the specified column in a line.
func replaceOperatorInLine(line string, mutation m.Mutation) string {
	if mutation.Column < 1 || mutation.Column > len(line) {
		return line
	}

	// Find and replace the operator
	runes := []rune(line)
	originalOp := mutation.OriginalOp.String()
	mutatedOp := mutation.MutatedOp.String()

	// Column is 1-indexed
	col := mutation.Column - 1

	// Check if the original operator is at this position
	if col+len(originalOp) <= len(runes) {
		opInLine := string(runes[col : col+len(originalOp)])
		if opInLine == originalOp {
			result := string(runes[:col]) + mutatedOp + string(runes[col+len(originalOp):])
			return result
		}
	}

	return line
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

// findProjectRoot searches for go.mod file walking up the directory tree.
func findProjectRoot(startPath string) (string, error) {
	dir := filepath.Dir(startPath)

	for {
		goModPath := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)

		if parent == dir {
			// Reached root without finding go.mod
			return "", fmt.Errorf("go.mod not found in any parent directory of %s", startPath)
		}

		dir = parent
	}
}

// copyDir recursively copies a directory tree.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Calculate relative path
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		// Skip common directories that don't need to be copied
		if info.IsDir() {
			baseName := filepath.Base(path)
			if baseName == ".git" || baseName == "vendor" || baseName == "node_modules" {
				return filepath.SkipDir
			}
		}

		targetPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(targetPath, info.Mode())
		}

		// Copy file
		return copyFile(path, targetPath, info.Mode())
	})
}

// copyFile copies a single file.
func copyFile(src, dst string, mode os.FileMode) error {
	// #nosec G304 - src is internal project file path, not user input
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}

	defer func() {
		if closeErr := sourceFile.Close(); closeErr != nil {
			_ = closeErr
		}
	}()

	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return err
	}

	// #nosec G304 - dst is internal destination path, not user input
	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}

	defer func() {
		if closeErr := destFile.Close(); closeErr != nil {
			_ = closeErr
		}
	}()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return err
	}

	return os.Chmod(dst, mode)
}

// runGoTest runs 'go test' on a specific test file in the given directory.
func runGoTest(workDir, testFile string) (string, error) {
	// Create context with timeout to avoid hanging tests
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "test", "-v", testFile)
	cmd.Dir = workDir

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	output := stdout.String() + stderr.String()

	return output, err
}
