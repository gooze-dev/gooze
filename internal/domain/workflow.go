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

type Workflow interface {
	GetSources(root m.Path) ([]m.Source, error)
}

type workflow struct{}

func NewWorkflow() Workflow {
	return &workflow{}
}

// GetSources walks the directory tree and identifies code scopes for mutation testing.
// It distinguishes between:
// - Global scope (const, var, type declarations) - for mutations like boolean literals, numbers
// - Init functions - for all mutation types
// - Regular functions - for function-specific mutations
func (w *workflow) GetSources(root m.Path) ([]m.Source, error) {
	rootPath := string(root)

	// Verify root exists
	if _, err := os.Stat(rootPath); err != nil {
		return nil, fmt.Errorf("root path error: %w", err)
	}

	var sources []m.Source
	fset := token.NewFileSet()

	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip non-Go files
		if info.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}

		// Parse the file
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if parseErr != nil {
			return fmt.Errorf("parse error in %s: %w", path, parseErr)
		}

		// Extract scopes from this file
		scopes := extractScopes(fset, file)

		// Skip files with no relevant scopes
		if len(scopes) == 0 {
			return nil
		}

		// Calculate file hash
		hash, hashErr := hashFile(path)
		if hashErr != nil {
			return fmt.Errorf("hash error for %s: %w", path, hashErr)
		}

		// Extract function lines for backward compatibility
		functionLines := extractFunctionLines(scopes)

		// Skip if no scopes at all (empty files, or only types)
		if len(scopes) == 0 {
			return nil
		}

		// For backward compatibility, only include in results if:
		// - Has functions/init (len(functionLines) > 0), OR
		// - Has global const/var declarations
		hasGlobals := hasGlobalScopes(scopes)
		if len(functionLines) == 0 && !hasGlobals {
			return nil
		}

		source := m.Source{
			Hash:   hash,
			Origin: m.Path(path),
			Test:   "", // TODO: implement test file detection
			Lines:  functionLines,
			Scopes: scopes,
		}

		sources = append(sources, source)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return sources, nil
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
	seen := make(map[int]bool)

	for _, scope := range scopes {
		// Only include function and init scopes, not global
		if scope.Type == m.ScopeFunction || scope.Type == m.ScopeInit {
			if !seen[scope.StartLine] {
				lines = append(lines, scope.StartLine)
				seen[scope.StartLine] = true
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
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
