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
		{x: "v", op: "==", y: `" "`},
		{x: "v", op: "==", y: "`a`"},
		{x: "len(s)", op: "==", y: "'a'"},
		{x: "max(n, 1)", op: "==", y: "0"},
		{x: "byteCount(s)", op: "==", y: "0"},
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

// comparisonMutantLines returns the changed line of each comparison mutant of
// the file src, trimmed and sorted.
func comparisonMutantLines(t *testing.T, src string) []string {
	t.Helper()

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var lines []string

	ast.Inspect(file, func(n ast.Node) bool {
		for _, mutation := range GenerateComparisonMutations(n, fset, []byte(src), rpt.Source{}) {
			original := strings.Split(src, "\n")
			for i, line := range strings.Split(string(mutation.MutatedCode), "\n") {
				if i < len(original) && line != original[i] {
					lines = append(lines, strings.TrimSpace(line))
				}
			}
		}

		return true
	})

	slices.Sort(lines)

	return lines
}

func TestGenerateComparisonMutations_SkipsEquivalentMutantsOfNonNegativeVariables(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		cond    string // the comparison, as written on its own line
		skipped string // the equivalent mutant; "" keeps all five
	}{
		{
			name: "unsigned parameter", cond: "u <= 0", skipped: "u == 0",
			src: "package p\nfunc f(u uint16) bool {\n\treturn u <= 0\n}\n",
		},
		{
			name: "byte parameter", cond: "b > 0", skipped: "b != 0",
			src: "package p\nfunc f(b byte) bool {\n\treturn b > 0\n}\n",
		},
		{
			name: "unsigned var", cond: "x == 0", skipped: "x <= 0",
			src: "package p\nfunc f() bool {\n\tvar x uint\n\treturn x == 0\n}\n",
		},
		{
			name: "length in a variable", cond: "n > 0", skipped: "n != 0",
			src: "package p\nfunc f(s []int) bool {\n\tn := len(s)\n\treturn n > 0\n}\n",
		},
		{
			name: "length in an if init", cond: "n > 0", skipped: "n != 0",
			src: "package p\nfunc f(s []int) bool {\n\tif n := len(s); n > 0 {\n\t\treturn true\n\t}\n\treturn false\n}\n",
		},
		{
			name: "capacity on the left", cond: "0 < c", skipped: "0 != c",
			src: "package p\nfunc f(s []int) bool {\n\tc := cap(s)\n\treturn 0 < c\n}\n",
		},
		{
			name: "uint8 parameter", cond: "u > 0", skipped: "u != 0",
			src: "package p\nfunc f(u uint8) bool {\n\treturn u > 0\n}\n",
		},
		{
			name: "uint32 parameter", cond: "u > 0", skipped: "u != 0",
			src: "package p\nfunc f(u uint32) bool {\n\treturn u > 0\n}\n",
		},
		{
			name: "uint64 parameter", cond: "u > 0", skipped: "u != 0",
			src: "package p\nfunc f(u uint64) bool {\n\treturn u > 0\n}\n",
		},
		{
			name: "uintptr parameter", cond: "u > 0", skipped: "u != 0",
			src: "package p\nfunc f(u uintptr) bool {\n\treturn u > 0\n}\n",
		},
		{
			name: "unsigned constant", cond: "k == 0", skipped: "k <= 0",
			src: "package p\nconst k uint = 3\nfunc f() bool {\n\treturn k == 0\n}\n",
		},
		{
			name: "type named like an unsigned type", cond: "u > 0",
			src: "package p\ntype uint16 int\nfunc f(u uint16) bool {\n\treturn u > 0\n}\n",
		},
		{
			name: "function", cond: "g > 0",
			src: "package p\nfunc f() bool {\n\treturn g > 0\n}\nfunc g() {}\n",
		},
		{
			name: "length defined after a name that sorts before", cond: "n > 0", skipped: "n != 0",
			src: "package p\nfunc f(s []int) bool {\n\tk, n := 5, len(s)\n\tk--\n\t_ = k\n\treturn n > 0\n}\n",
		},
		{
			name: "length defined before a name that sorts after", cond: "n > 0", skipped: "n != 0",
			src: "package p\nfunc f(s []int) bool {\n\tn, z := len(s), 5\n\tz--\n\t_ = z\n\treturn n > 0\n}\n",
		},
		{
			name: "not a length next to one that sorts after", cond: "n > 0",
			src: "package p\nfunc f(s []int) bool {\n\tn, z := -1, len(s)\n\t_ = z\n\treturn n > 0\n}\n",
		},
		{
			name: "not a length next to one that sorts before", cond: "n > 0",
			src: "package p\nfunc f(s []int) bool {\n\tk, n := len(s), -1\n\t_ = k\n\treturn n > 0\n}\n",
		},
		{
			name: "not a length next to one that sorts after it first", cond: "n > 0",
			src: "package p\nfunc f(s []int) bool {\n\tz, n := len(s), -1\n\t_ = z\n\treturn n > 0\n}\n",
		},
		{
			name: "variable from another file", cond: "x > 0",
			src: "package p\nfunc f() bool {\n\treturn x > 0\n}\n",
		},
		{
			name: "two values from one call", cond: "n > 0",
			src: "package p\nfunc f(s []int) bool {\n\tk, n := two(s)\n\t_ = k\n\treturn n > 0\n}\nfunc two(s []int) (int, int) { return len(s), -1 }\n",
		},
		{
			name: "length read with unary operators", cond: "n > 0", skipped: "n != 0",
			src: "package p\nfunc f(s []int) bool {\n\tn := len(s)\n\t_, _ = -n, ^n\n\treturn n > 0\n}\n",
		},
		{
			name: "same name written in other functions", cond: "n > 0", skipped: "n != 0",
			src: "package p\nfunc a(s []int) {\n\tn := len(s)\n\tn--\n\t_ = n\n}\nfunc f(s []int) bool {\n\tn := len(s)\n\treturn n > 0\n}\nfunc b(s []int) {\n\tn := len(s)\n\tn--\n\t_ = n\n}\n",
		},
		{
			name: "pointer written before the length", cond: "m > 0",
			src: "package p\nfunc set(p *int) { *p = 1 }\nfunc f(s []int) bool {\n\tm := len(s)\n\tset(&m)\n\treturn m > 0\n}\n",
		},
		{
			name: "length assigned by range", cond: "m > 0",
			src: "package p\nfunc f(s []int) bool {\n\tm := len(s)\n\tfor m = range s {\n\t}\n\treturn m > 0\n}\n",
		},
		{
			name: "length redefined by :=", cond: "m > 0",
			src: "package p\nfunc f(s []int) bool {\n\tm := len(s)\n\tm, err := -1, error(nil)\n\t_ = err\n\treturn m > 0\n}\n",
		},
		{
			name: "length decremented", cond: "m > 0",
			src: "package p\nfunc f(s []int) bool {\n\tm := len(s)\n\tm--\n\treturn m > 0\n}\n",
		},
		{
			name: "length reassigned", cond: "m > 0",
			src: "package p\nfunc f(s []int) bool {\n\tm := len(s)\n\tm = -1\n\treturn m > 0\n}\n",
		},
		{
			name: "length reassigned with an operator", cond: "m > 0",
			src: "package p\nfunc f(s []int) bool {\n\tm := len(s)\n\tm -= 2\n\treturn m > 0\n}\n",
		},
		{
			name: "length whose address is taken", cond: "m > 0",
			src: "package p\nfunc f(s []int) bool {\n\tm := len(s)\n\tdec(&m)\n\treturn m > 0\n}\nfunc dec(p *int) { *p-- }\n",
		},
		{
			name: "signed parameter", cond: "i > 0",
			src: "package p\nfunc f(i int) bool {\n\treturn i > 0\n}\n",
		},
		{
			name: "unsigned parameter shadowed by an int", cond: "u <= 0",
			src: "package p\nfunc f(u uint16) bool {\n\t{\n\t\tu := -1\n\t\treturn u <= 0\n\t}\n}\n",
		},
		{
			name: "not a length", cond: "k > 0",
			src: "package p\nfunc f(s []int) bool {\n\tk := s[0]\n\treturn k > 0\n}\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parts := strings.Fields(tt.cond)
			x, op, y := parts[0], parts[1], parts[2]

			var want []string
			for _, alt := range []string{"<", ">", "<=", ">=", "==", "!="} {
				if alt == op {
					continue
				}

				mutant := x + " " + alt + " " + y
				if mutant == tt.skipped {
					continue
				}

				line := strings.Replace(strings.TrimSpace(lineWith(tt.src, tt.cond)), tt.cond, mutant, 1)
				want = append(want, line)
			}

			slices.Sort(want)

			if got := comparisonMutantLines(t, tt.src); !slices.Equal(got, want) {
				t.Fatalf("mutants of %s:\n got %q\nwant %q", tt.cond, got, want)
			}
		})
	}
}

// lineWith returns the line of src containing s.
func lineWith(src, s string) string {
	for _, line := range strings.Split(src, "\n") {
		if strings.Contains(line, s) {
			return line
		}
	}

	return ""
}
