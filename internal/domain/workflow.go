// Package domain contains the core business logic for mutation testing.
package domain

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"

	m "github.com/mouse-blink/gooze/internal/model"
)

// Workflow defines the interface for mutation testing operations.
type Workflow interface {
	GetSources(roots ...m.Path) ([]m.Source, error)
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
// - Regular functions - for function-specific mutations
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

// scanPath scans a single path (with optional /... suffix) for Go source files
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

// parseRootPath extracts the root path and recursive flag from a path string
func parseRootPath(rootStr string) (path string, recursive bool) {
	if len(rootStr) >= 4 && rootStr[len(rootStr)-4:] == "/..." {
		return rootStr[:len(rootStr)-4], true
	}

	return rootStr, false
}

// handleDirectory determines if directory traversal should continue
func handleDirectory(path, rootStr string, recursive bool) error {
	if !recursive && path != rootStr {
		return filepath.SkipDir
	}

	return nil
}

// processFile parses and extracts scopes from a single Go file
func (w *workflow) processFile(path string, fset *token.FileSet) (m.Source, bool, error) {
	// Skip non-Go files
	if filepath.Ext(path) != ".go" {
		return m.Source{}, false, nil
	}

	file, parseErr := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if parseErr != nil {
		// Skip files with parse errors instead of failing the entire scan
		return m.Source{}, false, nil
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

	source := m.Source{
		Hash:   hash,
		Origin: m.Path(path),
		Test:   "", // TODO: implement test file detection
		Lines:  functionLines,
		Scopes: scopes,
	}

	return source, true, nil
}

// extractScopes analyzes an AST and returns all relevant code scopes
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

// extractFunctionLines extracts line numbers for functions only (backward compatibility)
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

// hasGlobalScopes checks if there are any global const/var scopes
func hasGlobalScopes(scopes []m.CodeScope) bool {
	for _, scope := range scopes {
		if scope.Type == m.ScopeGlobal {
			return true
		}
	}

	return false
}

// hashFile computes SHA-256 hash of a file
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
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
