package mutagens

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	m "github.com/mouse-blink/gooze/internal/model"
)

func TestGenerateStatementMutations_AssignmentDeletion(t *testing.T) {
	examplePath := filepath.Join("..", "..", "..", "examples", "statement", "main.go")
	content, err := os.ReadFile(examplePath)
	if err != nil {
		t.Fatalf("failed to read example file: %v", err)
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, examplePath, content, parser.ParseComments)
	if err != nil {
		t.Fatalf("failed to parse source: %v", err)
	}

	src := m.Source{
		Origin: &m.File{FullPath: m.Path(examplePath)},
	}

	var mutations []m.Mutation
	ast.Inspect(file, func(n ast.Node) bool {
		mutations = append(mutations, GenerateStatementMutations(n, fset, content, src)...)
		return true
	})

	// Should have deletions for assignments in assignments() function
	if len(mutations) < 3 {
		t.Fatalf("expected at least 3 mutations, got %d", len(mutations))
	}

	// Verify at least one assignment was deleted
	foundDeletion := false
	for _, mutation := range mutations {
		code := string(mutation.MutatedCode)
		// Check if any of the assignments are missing
		if !strings.Contains(code, "x := 10") || !strings.Contains(code, "y := 20") || !strings.Contains(code, "z := x + y") {
			foundDeletion = true
			break
		}
	}

	if !foundDeletion {
		t.Error("expected at least one assignment deletion mutation")
	}
}

func TestGenerateStatementMutations_ExpressionDeletion(t *testing.T) {
	examplePath := filepath.Join("..", "..", "..", "examples", "statement", "main.go")
	content, err := os.ReadFile(examplePath)
	if err != nil {
		t.Fatalf("failed to read example file: %v", err)
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, examplePath, content, parser.ParseComments)
	if err != nil {
		t.Fatalf("failed to parse source: %v", err)
	}

	src := m.Source{
		Origin: &m.File{FullPath: m.Path(examplePath)},
	}

	var mutations []m.Mutation
	ast.Inspect(file, func(n ast.Node) bool {
		mutations = append(mutations, GenerateStatementMutations(n, fset, content, src)...)
		return true
	})

	// Should have deletions for expression statements in expressions() function
	if len(mutations) < 2 {
		t.Fatalf("expected at least 2 mutations, got %d", len(mutations))
	}

	// Verify expression statements were deleted
	foundDeletedPrintln := false
	foundDeletedPrintf := false

	for _, mutation := range mutations {
		code := string(mutation.MutatedCode)
		if !strings.Contains(code, "Println") {
			foundDeletedPrintln = true
		}
		if !strings.Contains(code, "Printf") {
			foundDeletedPrintf = true
		}
	}

	if !foundDeletedPrintln {
		t.Error("expected Println deletion mutation")
	}
	if !foundDeletedPrintf {
		t.Error("expected Printf deletion mutation")
	}
}

func TestGenerateStatementMutations_DeferDeletion(t *testing.T) {
	examplePath := filepath.Join("..", "..", "..", "examples", "statement", "main.go")
	content, err := os.ReadFile(examplePath)
	if err != nil {
		t.Fatalf("failed to read example file: %v", err)
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, examplePath, content, parser.ParseComments)
	if err != nil {
		t.Fatalf("failed to parse source: %v", err)
	}

	src := m.Source{
		Origin: &m.File{FullPath: m.Path(examplePath)},
	}

	var mutations []m.Mutation
	ast.Inspect(file, func(n ast.Node) bool {
		mutations = append(mutations, GenerateStatementMutations(n, fset, content, src)...)
		return true
	})

	// Should have at least 1 deletion for defer statement
	if len(mutations) < 1 {
		t.Fatalf("expected at least 1 mutation, got %d", len(mutations))
	}

	// Verify defer was deleted
	foundDeferDeletion := false
	for _, mutation := range mutations {
		code := string(mutation.MutatedCode)
		if !strings.Contains(code, "defer cleanup()") && strings.Contains(code, "func deferStatements()") {
			foundDeferDeletion = true
			break
		}
	}

	if !foundDeferDeletion {
		t.Error("expected defer deletion mutation")
	}
}

func TestGenerateStatementMutations_GoDeletion(t *testing.T) {
	examplePath := filepath.Join("..", "..", "..", "examples", "statement", "main.go")
	content, err := os.ReadFile(examplePath)
	if err != nil {
		t.Fatalf("failed to read example file: %v", err)
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, examplePath, content, parser.ParseComments)
	if err != nil {
		t.Fatalf("failed to parse source: %v", err)
	}

	src := m.Source{
		Origin: &m.File{FullPath: m.Path(examplePath)},
	}

	var mutations []m.Mutation
	ast.Inspect(file, func(n ast.Node) bool {
		mutations = append(mutations, GenerateStatementMutations(n, fset, content, src)...)
		return true
	})

	// Should have at least 1 deletion for go statement
	if len(mutations) < 1 {
		t.Fatalf("expected at least 1 mutation, got %d", len(mutations))
	}

	// Verify go statement was deleted
	foundGoDeletion := false
	for _, mutation := range mutations {
		code := string(mutation.MutatedCode)
		if !strings.Contains(code, "go worker()") && strings.Contains(code, "func goroutines()") {
			foundGoDeletion = true
			break
		}
	}

	if !foundGoDeletion {
		t.Error("expected go statement deletion mutation")
	}
}

func TestGenerateStatementMutations_SendStatementDeletion(t *testing.T) {
	examplePath := filepath.Join("..", "..", "..", "examples", "statement", "main.go")
	content, err := os.ReadFile(examplePath)
	if err != nil {
		t.Fatalf("failed to read example file: %v", err)
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, examplePath, content, parser.ParseComments)
	if err != nil {
		t.Fatalf("failed to parse source: %v", err)
	}

	src := m.Source{
		Origin: &m.File{FullPath: m.Path(examplePath)},
	}

	var mutations []m.Mutation
	ast.Inspect(file, func(n ast.Node) bool {
		mutations = append(mutations, GenerateStatementMutations(n, fset, content, src)...)
		return true
	})

	// Should have at least 1 deletion for send statement
	if len(mutations) < 1 {
		t.Fatalf("expected at least 1 mutation, got %d", len(mutations))
	}

	// Verify send statement was deleted
	foundSendDeletion := false
	for _, mutation := range mutations {
		code := string(mutation.MutatedCode)
		if !strings.Contains(code, "ch <- 42") && strings.Contains(code, "func channels") {
			foundSendDeletion = true
			break
		}
	}

	if !foundSendDeletion {
		t.Error("expected channel send deletion mutation")
	}
}

func TestGenerateStatementMutations_OnlyDeletions(t *testing.T) {
	examplePath := filepath.Join("..", "..", "..", "examples", "statement", "main.go")
	content, err := os.ReadFile(examplePath)
	if err != nil {
		t.Fatalf("failed to read example file: %v", err)
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, examplePath, content, parser.ParseComments)
	if err != nil {
		t.Fatalf("failed to parse source: %v", err)
	}

	src := m.Source{
		Origin: &m.File{FullPath: m.Path(examplePath)},
	}

	var mutations []m.Mutation
	ast.Inspect(file, func(n ast.Node) bool {
		mutations = append(mutations, GenerateStatementMutations(n, fset, content, src)...)
		return true
	})

	// All mutations should be statement deletions (no operator mutations)
	if len(mutations) < 1 {
		t.Fatalf("expected at least 1 mutation, got %d", len(mutations))
	}

	// All mutations should be statement deletions
	for _, mutation := range mutations {
		if mutation.Type != m.MutationStatement {
			t.Errorf("expected mutation type MutationStatement, got %v", mutation.Type)
		}
	}
}
