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

func TestGenerateLoopMutations_BoundaryCondition(t *testing.T) {
	source := `package main

func foo(n int) int {
	sum := 0
	for i := 0; i < n; i++ {
		sum += i
	}
	return sum
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", source, 0)
	if err != nil {
		t.Fatalf("failed to parse source: %v", err)
	}

	src := m.Source{
		Origin: &m.File{FullPath: m.Path("test.go")},
	}

	var mutations []m.Mutation
	ast.Inspect(file, func(n ast.Node) bool {
		mutations = append(mutations, GenerateLoopMutations(n, fset, []byte(source), src)...)
		return true
	})

	if len(mutations) == 0 {
		t.Fatal("expected mutations, got none")
	}

	// Should have at least: boundary mutation (< to <=) and body removal
	if len(mutations) < 2 {
		t.Fatalf("expected at least 2 mutations, got %d", len(mutations))
	}

	// Check that we have a boundary mutation
	foundBoundaryMutation := false
	for _, mutation := range mutations {
		if mutation.Type != m.MutationLoop {
			t.Errorf("expected type MutationLoop, got %v", mutation.Type)
		}
		if strings.Contains(string(mutation.MutatedCode), "i <= n") {
			foundBoundaryMutation = true
		}
	}

	if !foundBoundaryMutation {
		t.Error("expected boundary mutation changing < to <=")
	}
}

func TestGenerateLoopMutations_RangeLoopBodyRemoval(t *testing.T) {
	examplePath := filepath.Join("..", "..", "..", "examples", "loops", "main.go")
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
		mutations = append(mutations, GenerateLoopMutations(n, fset, content, src)...)
		return true
	})

	// Should have mutations for range loops
	foundRangeBodyRemoval := false
	for _, mutation := range mutations {
		code := string(mutation.MutatedCode)
		// Check if range loop body was removed (empty body but range statement remains)
		if strings.Contains(code, "for _, item := range items {") &&
			!strings.Contains(code, "sum += item") {
			foundRangeBodyRemoval = true
			break
		}
	}

	if !foundRangeBodyRemoval {
		t.Error("expected range loop body removal mutation")
	}
}

func TestGenerateLoopMutations_BreakRemoval(t *testing.T) {
	examplePath := filepath.Join("..", "..", "..", "examples", "loops", "main.go")
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
		mutations = append(mutations, GenerateLoopMutations(n, fset, content, src)...)
		return true
	})

	// Should have mutation for break removal
	foundBreakRemoval := false
	for _, mutation := range mutations {
		code := string(mutation.MutatedCode)
		// Check if break statement was removed
		if strings.Contains(code, "func loopWithBreak") &&
			strings.Contains(code, "if i == 5") &&
			!strings.Contains(code, "break") {
			foundBreakRemoval = true
			break
		}
	}

	if !foundBreakRemoval {
		t.Error("expected break statement removal mutation")
	}
}

func TestGenerateLoopMutations_ContinueRemoval(t *testing.T) {
	examplePath := filepath.Join("..", "..", "..", "examples", "loops", "main.go")
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
		mutations = append(mutations, GenerateLoopMutations(n, fset, content, src)...)
		return true
	})

	// Should have mutation for continue removal
	foundContinueRemoval := false
	for _, mutation := range mutations {
		code := string(mutation.MutatedCode)
		// Check if continue statement was removed
		if strings.Contains(code, "func loopWithContinue") &&
			strings.Contains(code, "if i%2 == 0") &&
			!strings.Contains(code, "continue") {
			foundContinueRemoval = true
			break
		}
	}

	if !foundContinueRemoval {
		t.Error("expected continue statement removal mutation")
	}
}

func TestGenerateLoopMutations_LoopBodyRemoval(t *testing.T) {
	source := `package main

func bar() int {
	sum := 0
	for i := 0; i < 10; i++ {
		sum += i
		sum *= 2
	}
	return sum
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", source, 0)
	if err != nil {
		t.Fatalf("failed to parse source: %v", err)
	}

	src := m.Source{
		Origin: &m.File{FullPath: m.Path("test.go")},
	}

	var mutations []m.Mutation
	ast.Inspect(file, func(n ast.Node) bool {
		mutations = append(mutations, GenerateLoopMutations(n, fset, []byte(source), src)...)
		return true
	})

	// Should have body removal mutation
	foundBodyRemoval := false
	for _, mutation := range mutations {
		code := string(mutation.MutatedCode)
		// Check if loop body was removed (empty block)
		if strings.Contains(code, "for i := 0; i < 10; i++ {") &&
			!strings.Contains(code, "sum += i") &&
			!strings.Contains(code, "sum *= 2") {
			foundBodyRemoval = true
			break
		}
	}

	if !foundBodyRemoval {
		t.Error("expected loop body removal mutation")
	}
}

func TestGenerateLoopMutations_NestedLoopBoundaries(t *testing.T) {
	source := `package main

func nested(n int) int {
	sum := 0
	for i := 0; i < n; i++ {
		for j := 0; j <= i; j++ {
			sum++
		}
	}
	return sum
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", source, 0)
	if err != nil {
		t.Fatalf("failed to parse source: %v", err)
	}

	src := m.Source{
		Origin: &m.File{FullPath: m.Path("test.go")},
	}

	var mutations []m.Mutation
	ast.Inspect(file, func(n ast.Node) bool {
		mutations = append(mutations, GenerateLoopMutations(n, fset, []byte(source), src)...)
		return true
	})

	// Should have boundary mutations for both loops
	foundOuterBoundary := false
	foundInnerBoundary := false

	for _, mutation := range mutations {
		code := string(mutation.MutatedCode)
		if strings.Contains(code, "i <= n") && strings.Contains(code, "j <= i") {
			foundOuterBoundary = true
		}
		if strings.Contains(code, "i < n") && strings.Contains(code, "j < i") {
			foundInnerBoundary = true
		}
	}

	if !foundOuterBoundary {
		t.Error("expected outer loop boundary mutation")
	}
	if !foundInnerBoundary {
		t.Error("expected inner loop boundary mutation")
	}
}

func TestGenerateLoopMutations_RecursiveCallRemoval(t *testing.T) {
	examplePath := filepath.Join("..", "..", "..", "examples", "loops", "main.go")
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
		mutations = append(mutations, GenerateLoopMutations(n, fset, content, src)...)
		return true
	})

	// Should have mutations for recursive calls
	foundRecursiveMutation := false
	for _, mutation := range mutations {
		code := string(mutation.MutatedCode)
		// Check if recursive call was replaced with 0
		if strings.Contains(code, "func factorialRecursive") &&
			strings.Contains(code, "return n * 0") {
			foundRecursiveMutation = true
			break
		}
	}

	if !foundRecursiveMutation {
		t.Error("expected recursive call mutation")
	}
}
