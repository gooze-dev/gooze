package mutagens

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strings"
	"testing"

	m "gooze.dev/pkg/gooze/internal/model"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

type generator func(ast.Node, *token.FileSet, []byte, rpt.Source) []m.Mutation

// mutantsOf parses body as the body of a function and returns, sorted, the
// lines each mutation of gen changes, joined by "; ". A mutation that only
// deletes lines shows them as "-" and the deleted lines.
func mutantsOf(t *testing.T, gen generator, body string) []string {
	t.Helper()

	code := "package p\n\nfunc f(a, b, n int, s string, c chan int) {\n" + body + "\n}\n"
	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, "p.go", code, parser.AllErrors)
	if err != nil {
		t.Fatalf("parse %q: %v", body, err)
	}

	var mutants []string

	ast.Inspect(file, func(n ast.Node) bool {
		for _, mutation := range gen(n, fset, []byte(code), rpt.Source{}) {
			mutants = append(mutants, changedLines(code, string(mutation.MutatedCode)))
		}

		return true
	})

	slices.Sort(mutants)

	return mutants
}

// changedLines returns the lines of mutated that differ from original, trimmed
// and joined by "; ", or "-" and the deleted lines when mutated only deletes.
func changedLines(original, mutated string) string {
	a, b := strings.Split(original, "\n"), strings.Split(mutated, "\n")

	prefix := 0
	for prefix < len(a) && prefix < len(b) && a[prefix] == b[prefix] {
		prefix++
	}

	suffix := 0
	for suffix < len(a)-prefix && suffix < len(b)-prefix && a[len(a)-1-suffix] == b[len(b)-1-suffix] {
		suffix++
	}

	trimmed := func(lines []string) string {
		for i, line := range lines {
			lines[i] = strings.TrimSpace(line)
		}

		return strings.Join(lines, "; ")
	}

	if changed := b[prefix : len(b)-suffix]; len(changed) > 0 {
		return trimmed(changed)
	}

	return "-" + trimmed(a[prefix:len(a)-suffix])
}

// generateAll returns every mutation gen generates for the file code, and
// fails unless there is at least one.
func generateAll(t *testing.T, gen generator, code string) []m.Mutation {
	t.Helper()

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, "p.go", code, parser.AllErrors)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var mutations []m.Mutation

	ast.Inspect(file, func(n ast.Node) bool {
		mutations = append(mutations, gen(n, fset, []byte(code), rpt.Source{})...)
		return true
	})

	if len(mutations) == 0 {
		t.Fatal("expected mutations")
	}

	return mutations
}

// inspectAll calls gen on every node of root, as gooze walks a file, and
// returns the mutations in walk order.
func inspectAll(gen generator, root ast.Node, fset *token.FileSet, content []byte, source rpt.Source) []m.Mutation {
	var mutations []m.Mutation

	ast.Inspect(root, func(n ast.Node) bool {
		mutations = append(mutations, gen(n, fset, content, source)...)
		return true
	})

	return mutations
}
