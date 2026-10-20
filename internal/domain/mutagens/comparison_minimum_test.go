package mutagens

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strings"
	"testing"

	rpt "gooze.dev/pkg/gooze/pkg/report"
)

// comparisonMutants returns the expression each comparison mutant of expr
// turns it into, sorted.
func comparisonMutants(t *testing.T, expr string) []string {
	t.Helper()

	const prefix = "package main\nfunc f(v string, n int, s []int) bool { return "

	code := prefix + expr + " }\n"
	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, "test.go", code, parser.AllErrors)
	if err != nil {
		t.Fatalf("parse %q: %v", expr, err)
	}

	var mutants []string

	ast.Inspect(file, func(n ast.Node) bool {
		for _, mutation := range GenerateComparisonMutations(n, fset, []byte(code), rpt.Source{}) {
			mutated := strings.TrimPrefix(string(mutation.MutatedCode), prefix)
			mutants = append(mutants, strings.TrimSuffix(mutated, " }\n"))
		}

		return true
	})

	slices.Sort(mutants)

	return mutants
}

// allComparisonMutants returns the five mutants of "x op y" for every
// comparison operator but op, sorted.
func allComparisonMutants(x, op, y string) []string {
	var mutants []string

	for _, alt := range []string{"<", ">", "<=", ">=", "==", "!="} {
		if alt != op {
			mutants = append(mutants, x+" "+alt+" "+y)
		}
	}

	slices.Sort(mutants)

	return mutants
}

func without(mutants []string, skipped string) []string {
	return slices.DeleteFunc(slices.Clone(mutants), func(m string) bool { return m == skipped })
}

func TestGenerateComparisonMutations_SkipsEquivalentMutantsAgainstMinimum(t *testing.T) {
	tests := []struct {
		x, op, y string
		skipped  string // the equivalent mutant; "" keeps all five
	}{
		// Minimum on the right.
		{x: "v", op: "==", y: `""`, skipped: `v <= ""`},
		{x: "v", op: "!=", y: `""`, skipped: `v > ""`},
		{x: "v", op: ">", y: `""`, skipped: `v != ""`},
		{x: "v", op: "<=", y: `""`, skipped: `v == ""`},
		{x: "v", op: "==", y: "``", skipped: "v <= ``"},
		{x: "len(s)", op: "==", y: "0", skipped: "len(s) <= 0"},
		{x: "len(s)", op: "!=", y: "0", skipped: "len(s) > 0"},
		{x: "cap(s)", op: ">", y: "0", skipped: "cap(s) != 0"},
		{x: "cap(s)", op: "<=", y: "0", skipped: "cap(s) == 0"},
		// Minimum on the left.
		{x: `""`, op: "==", y: "v", skipped: `"" >= v`},
		{x: `""`, op: "!=", y: "v", skipped: `"" < v`},
		{x: `""`, op: "<", y: "v", skipped: `"" != v`},
		{x: `""`, op: ">=", y: "v", skipped: `"" == v`},
		{x: "0", op: "==", y: "len(s)", skipped: "0 >= len(s)"},
		{x: "0", op: "<", y: "cap(s)", skipped: "0 != cap(s)"},
		// The other operators have no equivalent mutant.
		{x: "v", op: "<", y: `""`},
		{x: "v", op: ">=", y: `""`},
		{x: `""`, op: ">", y: "v"},
		{x: `""`, op: "<=", y: "v"},
		{x: "len(s)", op: "<", y: "0"},
		// Not a known minimum.
		{x: "v", op: "==", y: `"a"`},
		{x: "n", op: "==", y: "0"},
		{x: "n", op: "!=", y: "0"},
		{x: "0", op: "<", y: "n"},
		{x: "len(s)", op: "==", y: "1"},
		{x: "len(s)", op: "!=", y: "-0"},
		{x: "f(s)", op: "!=", y: "0"},
	}

	for _, tt := range tests {
		expr := tt.x + " " + tt.op + " " + tt.y
		t.Run(expr, func(t *testing.T) {
			want := allComparisonMutants(tt.x, tt.op, tt.y)
			if tt.skipped != "" {
				want = without(want, tt.skipped)
			}

			if got := comparisonMutants(t, expr); !slices.Equal(got, want) {
				t.Fatalf("mutants of %s = %q, want %q", expr, got, want)
			}
		})
	}
}
