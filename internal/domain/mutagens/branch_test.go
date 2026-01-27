package mutagens

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	m "github.com/mouse-blink/gooze/internal/model"
)

func TestGenerateBranchMutations_IfStatement(t *testing.T) {
	source := `package main

func foo(x int) int {
	if x > 5 {
		return 10
	}
	return 0
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

	mutations := GenerateBranchMutations(file, fset, []byte(source), src)

	if len(mutations) == 0 {
		t.Fatal("expected mutations, got none")
	}

	// Should generate mutations: 3 for condition (inverted, forced true, false) + 1 to remove if block
	expectedMin := 4
	if len(mutations) < expectedMin {
		t.Fatalf("expected at least %d mutations for if statement, got %d", expectedMin, len(mutations))
	}

	// Check mutation types
	for i, mutation := range mutations {
		if mutation.Type != m.MutationBranch {
			t.Errorf("mutation %d: expected type MutationBranch, got %v", i, mutation.Type)
		}
		if len(mutation.ID) == 0 {
			t.Errorf("mutation %d: expected non-empty ID", i)
		}
		if len(mutation.MutatedCode) == 0 {
			t.Errorf("mutation %d: expected non-empty mutated code", i)
		}
	}

	// Check that mutations include expected variations
	foundInverted := false
	foundTrue := false
	foundFalse := false
	foundRemoved := false

	for _, mutation := range mutations {
		code := string(mutation.MutatedCode)
		if strings.Contains(code, "!(x > 5)") {
			foundInverted = true
		}
		if strings.Contains(code, "if true {") {
			foundTrue = true
		}
		if strings.Contains(code, "if false {") {
			foundFalse = true
		}
		// Check if the if block was removed (no "if x" but still has "return 0")
		if !strings.Contains(code, "if x") && !strings.Contains(code, "if true") &&
			!strings.Contains(code, "if false") && !strings.Contains(code, "!(x") &&
			strings.Contains(code, "return 0") {
			foundRemoved = true
		}
	}

	if !foundInverted {
		t.Error("expected mutation with inverted condition")
	}
	if !foundTrue {
		t.Error("expected mutation with forced true")
	}
	if !foundFalse {
		t.Error("expected mutation with forced false")
	}
	if !foundRemoved {
		t.Error("expected mutation with if block removed")
	}
}

func TestGenerateBranchMutations_ForLoop(t *testing.T) {
	source := `package main

func bar() {
	for i := 0; i < 10; i++ {
		println(i)
	}
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

	mutations := GenerateBranchMutations(file, fset, []byte(source), src)

	if len(mutations) == 0 {
		t.Fatal("expected mutations for for loop, got none")
	}

	// Should generate 3 mutations for the loop condition
	if len(mutations) != 3 {
		t.Fatalf("expected 3 mutations for for loop, got %d", len(mutations))
	}

	for i, mutation := range mutations {
		if mutation.Type != m.MutationBranch {
			t.Errorf("mutation %d: expected type MutationBranch, got %v", i, mutation.Type)
		}
	}

	code0 := string(mutations[0].MutatedCode)
	if !strings.Contains(code0, "!(i < 10)") {
		t.Errorf("expected inverted condition in for loop mutation, got: %s", code0)
	}
}

func TestGenerateBranchMutations_MultipleConditions(t *testing.T) {
	source := `package main

func baz(a, b int) int {
	if a > 0 {
		if b < 10 {
			return a + b
		}
	}
	return 0
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

	mutations := GenerateBranchMutations(file, fset, []byte(source), src)

	// Should generate 6 mutations: 3 for each if statement (inverted, true, false) + 1 removal each
	if len(mutations) != 8 {
		t.Fatalf("expected 8 mutations (2 if statements with nested structure), got %d", len(mutations))
	}

	for i, mutation := range mutations {
		if mutation.Type != m.MutationBranch {
			t.Errorf("mutation %d: expected type MutationBranch, got %v", i, mutation.Type)
		}
	}
}

func TestGenerateBranchMutations_NoConditions(t *testing.T) {
	source := `package main

func simple() int {
	x := 5
	return x + 10
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

	mutations := GenerateBranchMutations(file, fset, []byte(source), src)

	if len(mutations) != 0 {
		t.Fatalf("expected no mutations for code without conditionals, got %d", len(mutations))
	}
}

func TestGenerateBranchMutations_ComplexCondition(t *testing.T) {
	source := `package main

func complex(x, y int) bool {
	if x > 0 && y < 100 {
		return true
	}
	return false
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

	mutations := GenerateBranchMutations(file, fset, []byte(source), src)

	if len(mutations) != 4 {
		t.Fatalf("expected 4 mutations for complex condition (3 condition + 1 removal), got %d", len(mutations))
	}

	code0 := string(mutations[0].MutatedCode)
	// Should wrap the entire complex condition in parentheses and negate
	if !strings.Contains(code0, "!(x > 0 && y < 100)") {
		t.Errorf("expected negated complex condition, got: %s", code0)
	}
}

func TestInvertCondition_PreservesFormatting(t *testing.T) {
	source := `package main

func test() {
	if true {
		println("test")
	}
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", source, 0)
	if err != nil {
		t.Fatalf("failed to parse source: %v", err)
	}

	var ifStmt *ast.IfStmt
	ast.Inspect(file, func(n ast.Node) bool {
		if stmt, ok := n.(*ast.IfStmt); ok {
			ifStmt = stmt
			return false
		}
		return true
	})

	if ifStmt == nil {
		t.Fatal("could not find if statement")
	}

	src := m.Source{
		Origin: &m.File{FullPath: m.Path("test.go")},
	}

	mutations := invertCondition(ifStmt.Cond, fset, []byte(source), src)

	if len(mutations) != 3 {
		t.Fatalf("expected 3 mutations, got %d", len(mutations))
	}

	// Verify each mutation is valid Go code
	for i, mutation := range mutations {
		_, err := parser.ParseFile(token.NewFileSet(), "mutated.go", mutation.MutatedCode, 0)
		if err != nil {
			t.Errorf("mutation %d produced invalid Go code: %v\n%s", i, err, string(mutation.MutatedCode))
		}
	}
}

func TestGenerateBranchMutations_IfElseStatement(t *testing.T) {
	source := `package main

func test(x int) int {
	if x > 0 {
		return 1
	} else {
		return -1
	}
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

	mutations := GenerateBranchMutations(file, fset, []byte(source), src)

	// Should have: 3 condition mutations + 1 remove if block + 1 remove else block
	expectedMin := 5
	if len(mutations) < expectedMin {
		t.Fatalf("expected at least %d mutations for if-else, got %d", expectedMin, len(mutations))
	}

	foundRemovedIf := false
	foundRemovedElse := false

	for _, mutation := range mutations {
		code := string(mutation.MutatedCode)

		// Check for mutation that removes if block (keeps else)
		if !strings.Contains(code, "if x > 0") && strings.Contains(code, "return -1") && !strings.Contains(code, "return 1") {
			foundRemovedIf = true
		}

		// Check for mutation that removes else block (keeps if)
		if strings.Contains(code, "if") && strings.Contains(code, "return 1") && !strings.Contains(code, "else") && !strings.Contains(code, "return -1") {
			foundRemovedElse = true
		}
	}

	if !foundRemovedIf {
		t.Error("expected mutation with if block removed (keeping else)")
	}
	if !foundRemovedElse {
		t.Error("expected mutation with else block removed")
	}
}

func TestGenerateBranchMutations_ElseIfStatement(t *testing.T) {
	source := `package main

func test(x int) string {
	if x > 10 {
		return "high"
	} else if x > 0 {
		return "low"
	} else {
		return "negative"
	}
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

	mutations := GenerateBranchMutations(file, fset, []byte(source), src)

	// Should have mutations for both if statements (outer and else if)
	if len(mutations) < 8 {
		t.Fatalf("expected at least 8 mutations for else-if chain, got %d", len(mutations))
	}

	// Verify all mutations are valid
	for i, mutation := range mutations {
		if mutation.Type != m.MutationBranch {
			t.Errorf("mutation %d: expected type MutationBranch", i)
		}
	}
}

func TestGenerateBranchMutations_SwitchStatement(t *testing.T) {
	source := `package main

func test(x int) string {
	switch x {
	case 1:
		return "one"
	case 2:
		return "two"
	default:
		return "other"
	}
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

	mutations := GenerateBranchMutations(file, fset, []byte(source), src)

	// Should have mutations for each case body (3 cases)
	if len(mutations) < 3 {
		t.Fatalf("expected at least 3 mutations for switch statement, got %d", len(mutations))
	}

	foundRemovedCase := false
	for _, mutation := range mutations {
		code := string(mutation.MutatedCode)

		// Check that at least one case body was removed
		if strings.Contains(code, "case 1:") && !strings.Contains(code, `return "one"`) {
			foundRemovedCase = true
		}
	}

	if !foundRemovedCase {
		t.Error("expected mutation with case body removed")
	}
}

func TestRemoveCaseBody_PreservesStructure(t *testing.T) {
	source := `package main

func test(x int) {
	switch x {
	case 1:
		println("one")
		println("first")
	case 2:
		println("two")
	}
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

	mutations := GenerateBranchMutations(file, fset, []byte(source), src)

	if len(mutations) < 2 {
		t.Fatalf("expected at least 2 mutations, got %d", len(mutations))
	}

	// Verify mutations are syntactically valid
	for i, mutation := range mutations {
		_, err := parser.ParseFile(token.NewFileSet(), "mutated.go", mutation.MutatedCode, 0)
		if err != nil {
			t.Errorf("mutation %d produced invalid Go code: %v\n%s", i, err, string(mutation.MutatedCode))
		}
	}
}

func TestGenerateBranchMutations_NestedIf(t *testing.T) {
	source := `package main

func test(a, b int) int {
	if a > 0 {
		if b > 0 {
			return 1
		}
		return 2
	}
	return 3
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

	mutations := GenerateBranchMutations(file, fset, []byte(source), src)

	// Should have mutations for both outer and inner if statements
	// Each if gets: 3 condition mutations + 1 remove if block
	expectedMin := 8
	if len(mutations) < expectedMin {
		t.Fatalf("expected at least %d mutations for nested ifs, got %d", expectedMin, len(mutations))
	}
}
