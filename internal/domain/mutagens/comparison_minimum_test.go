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

func TestGenerateComparisonMutations_SkipsEquivalentMutantsOfNonNegativeCallsAndCounters(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		cond    string // the comparison, as written on its own line
		skipped string // the equivalent mutant; "" keeps all five
	}{
		// Standard-library functions that never return a negative value.
		{
			name: "strings.Count", cond: "strings.Count(s, \"a\") > 0", skipped: "strings.Count(s, \"a\") != 0",
			src: "package p\nimport \"strings\"\nfunc f(s string) bool {\n\treturn strings.Count(s, \"a\") > 0\n}\n",
		},
		{
			name: "utf8.RuneCountInString", cond: "utf8.RuneCountInString(s) == 0", skipped: "utf8.RuneCountInString(s) <= 0",
			src: "package p\nimport \"unicode/utf8\"\nfunc f(s string) bool {\n\treturn utf8.RuneCountInString(s) == 0\n}\n",
		},
		{
			name: "flag.NArg", cond: "flag.NArg() == 0", skipped: "flag.NArg() <= 0",
			src: "package p\nimport \"flag\"\nfunc f() bool {\n\treturn flag.NArg() == 0\n}\n",
		},
		{
			name: "renamed import", cond: "str.Count(s, \"a\") > 0", skipped: "str.Count(s, \"a\") != 0",
			src: "package p\nimport str \"strings\"\nfunc f(s string) bool {\n\treturn str.Count(s, \"a\") > 0\n}\n",
		},
		{
			name: "function that can return -1", cond: "utf8.RuneLen(r) > 0",
			src: "package p\nimport \"unicode/utf8\"\nfunc f(r rune) bool {\n\treturn utf8.RuneLen(r) > 0\n}\n",
		},
		{
			name: "strings.Index", cond: "strings.Index(s, \"a\") == 0",
			src: "package p\nimport \"strings\"\nfunc f(s string) bool {\n\treturn strings.Index(s, \"a\") == 0\n}\n",
		},
		{
			name: "package name shadowed by a variable", cond: "strings.Count(s, \"a\") > 0",
			src: "package p\nimport \"strings\"\ntype counter struct{}\nfunc (counter) Count(string, string) int { return -1 }\nfunc f(s string) bool {\n\t_ = strings.ToUpper\n\t{\n\t\tstrings := counter{}\n\t\treturn strings.Count(s, \"a\") > 0\n\t}\n}\n",
		},
		{
			name: "Count of a package that isn't imported", cond: "strings.Count(s, \"a\") > 0",
			src: "package p\nfunc f(s string) bool {\n\treturn strings.Count(s, \"a\") > 0\n}\n",
		},
		// Methods, when the receiver's type is visible.
		{
			name: "Builder variable", cond: "b.Len() == 0", skipped: "b.Len() <= 0",
			src: "package p\nimport \"strings\"\nfunc f() bool {\n\tvar b strings.Builder\n\treturn b.Len() == 0\n}\n",
		},
		{
			name: "Buffer pointer parameter", cond: "b.Len() > 0", skipped: "b.Len() != 0",
			src: "package p\nimport \"bytes\"\nfunc f(b *bytes.Buffer) bool {\n\treturn b.Len() > 0\n}\n",
		},
		{
			name: "Builder from a composite literal", cond: "b.Cap() > 0", skipped: "b.Cap() != 0",
			src: "package p\nimport \"strings\"\nfunc f() bool {\n\tb := &strings.Builder{}\n\treturn b.Cap() > 0\n}\n",
		},
		{
			name: "FlagSet from NewFlagSet", cond: "fs.NArg() == 0", skipped: "fs.NArg() <= 0",
			src: "package p\nimport \"flag\"\nfunc f() bool {\n\tfs := flag.NewFlagSet(\"x\", flag.ContinueOnError)\n\treturn fs.NArg() == 0\n}\n",
		},
		{
			name: "Reader from strings.NewReader", cond: "r.Len() == 0", skipped: "r.Len() <= 0",
			src: "package p\nimport \"strings\"\nfunc f(s string) bool {\n\tr := strings.NewReader(s)\n\treturn r.Len() == 0\n}\n",
		},
		{
			name: "Len of an own type", cond: "b.Len() == 0",
			src: "package p\ntype buf struct{}\nfunc (buf) Len() int { return -1 }\nfunc f(b buf) bool {\n\treturn b.Len() == 0\n}\n",
		},
		{
			name: "Len of a field", cond: "s.b.Len() == 0",
			src: "package p\nimport \"strings\"\ntype t struct{ b strings.Builder }\nfunc f(s *t) bool {\n\treturn s.b.Len() == 0\n}\n",
		},
		{
			name: "a method the allowlist doesn't name", cond: "b.Grow(1) == 0",
			src: "package p\nimport \"strings\"\nfunc f(b *strings.Builder) bool {\n\treturn b.Grow(1) == 0\n}\n",
		},
		{
			name: "Builder from new", cond: "b.Len() == 0",
			src: "package p\nimport \"strings\"\nfunc f() bool {\n\tb := new(strings.Builder)\n\treturn b.Len() == 0\n}\n",
		},
		{
			name: "constructor called on a field", cond: "r.Len() == 0",
			src: "package p\ntype t struct{ x x }\ntype x struct{}\nfunc (x) NewReader(string) *reader { return nil }\ntype reader struct{}\nfunc (*reader) Len() int { return -1 }\nfunc f(v t, s string) bool {\n\tr := v.x.NewReader(s)\n\treturn r.Len() == 0\n}\n",
		},
		{
			name: "a field compared with 0", cond: "v.n == 0",
			src: "package p\ntype t struct{ n int }\nfunc f(v t) bool {\n\treturn v.n == 0\n}\n",
		},
		// Counters that only go up.
		{
			name: "counter incremented", cond: "c > 0", skipped: "c != 0",
			src: "package p\nfunc f(s []int) bool {\n\tc := 0\n\tfor range s {\n\t\tc++\n\t}\n\treturn c > 0\n}\n",
		},
		{
			name: "counter increased by a constant", cond: "c == 0", skipped: "c <= 0",
			src: "package p\nfunc f(s []int) bool {\n\tc := 0\n\tfor range s {\n\t\tc += 2\n\t}\n\treturn c == 0\n}\n",
		},
		{
			name: "counter declared with var", cond: "c == 0", skipped: "c <= 0",
			src: "package p\nfunc f(s []int) bool {\n\tvar c int\n\tfor range s {\n\t\tc++\n\t}\n\treturn c == 0\n}\n",
		},
		{
			name: "length increased", cond: "n > 0", skipped: "n != 0",
			src: "package p\nfunc f(s []int) bool {\n\tn := len(s)\n\tn++\n\treturn n > 0\n}\n",
		},
		{
			name: "counter from a non-negative call", cond: "n > 0", skipped: "n != 0",
			src: "package p\nimport \"strings\"\nfunc f(s string) bool {\n\tn := strings.Count(s, \"a\")\n\tn++\n\treturn n > 0\n}\n",
		},
		{
			name: "counter decremented", cond: "c > 0",
			src: "package p\nfunc f(s []int) bool {\n\tc := 0\n\tc++\n\tc--\n\treturn c > 0\n}\n",
		},
		{
			name: "counter decreased", cond: "c > 0",
			src: "package p\nfunc f() bool {\n\tc := 0\n\tc -= 1\n\treturn c > 0\n}\n",
		},
		{
			name: "counter increased by a variable", cond: "c > 0",
			src: "package p\nfunc f(k int) bool {\n\tc := 0\n\tc += k\n\treturn c > 0\n}\n",
		},
		{
			name: "counter assigned a constant", cond: "c > 0",
			src: "package p\nfunc f() bool {\n\tc := 0\n\tc = 5\n\treturn c > 0\n}\n",
		},
		{
			name: "counter whose address is taken", cond: "c > 0",
			src: "package p\nfunc set(p *int) { *p = -1 }\nfunc f() bool {\n\tc := 0\n\tset(&c)\n\treturn c > 0\n}\n",
		},
		{
			name: "counter starting negative", cond: "c > 0",
			src: "package p\nfunc f() bool {\n\tc := -1\n\tc++\n\treturn c > 0\n}\n",
		},
		{
			name: "counter starting from a parameter", cond: "c > 0",
			src: "package p\nfunc f(k int) bool {\n\tc := k\n\tc++\n\treturn c > 0\n}\n",
		},
		{
			name: "package-level counter", cond: "c > 0",
			src: "package p\nvar c int\nfunc f() bool {\n\tc++\n\treturn c > 0\n}\n",
		},
		{
			name: "var with a value", cond: "c > 0", skipped: "c != 0",
			src: "package p\nfunc f() bool {\n\tvar c = 0\n\tc++\n\treturn c > 0\n}\n",
		},
		{
			name: "var after a name that sorts before it", cond: "c > 0", skipped: "c != 0",
			src: "package p\nfunc f() bool {\n\tvar a, c = -1, 0\n\t_ = a\n\tc++\n\treturn c > 0\n}\n",
		},
		{
			name: "var after a name that sorts after it", cond: "c > 0", skipped: "c != 0",
			src: "package p\nfunc f() bool {\n\tvar z, c = -1, 0\n\t_ = z\n\tc++\n\treturn c > 0\n}\n",
		},
		{
			name: "var given its value by a call", cond: "c > 0",
			src: "package p\nfunc two() (int, int) { return -1, -1 }\nfunc f() bool {\n\tvar a, c = two()\n\t_ = a\n\tc++\n\treturn c > 0\n}\n",
		},
		{
			name: "float counter", cond: "c > 0", skipped: "c != 0",
			src: "package p\nfunc f() bool {\n\tc := 0.5\n\tc += 1.5\n\treturn c > 0\n}\n",
		},
		{
			name: "package-level counter declared after the function", cond: "c > 0",
			src: "package p\nfunc f() bool {\n\tc++\n\treturn c > 0\n}\nvar c int\n",
		},
		{
			name: "var with a negative value", cond: "c > 0",
			src: "package p\nfunc f() bool {\n\tvar c = -1\n\tc++\n\treturn c > 0\n}\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x, op, y := splitComparison(tt.cond)

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

// splitComparison splits "x op y" at its comparison operator; x may contain
// spaces (a call with several arguments).
func splitComparison(cond string) (x, op, y string) {
	for _, candidate := range []string{" == ", " != ", " <= ", " >= ", " < ", " > "} {
		if i := strings.LastIndex(cond, candidate); i >= 0 {
			return cond[:i], strings.TrimSpace(candidate), cond[i+len(candidate):]
		}
	}

	return cond, "", ""
}

// TestNonNegativeAllowlists checks that every entry of the allowlists makes 0
// the minimum of its call, so a typo or a removed entry shows.
func TestNonNegativeAllowlists(t *testing.T) {
	type check struct{ name, src, cond, skipped string }

	var checks []check

	for fn := range nonNegativeFuncs {
		path, name := fn[0], fn[1]
		pkg := path[strings.LastIndex(path, "/")+1:]
		call := pkg + "." + name + "(x)"
		checks = append(checks, check{
			name: path + "." + name, cond: call + " == 0", skipped: call + " <= 0",
			src: "package p\nimport \"" + path + "\"\nfunc f(x int) bool {\n\treturn " + call + " == 0\n}\n",
		})
	}

	for method := range nonNegativeMethods {
		path, typ := splitType(method[0])
		pkg := path[strings.LastIndex(path, "/")+1:]
		call := "r." + method[1] + "()"
		checks = append(checks, check{
			name: method[0] + "." + method[1], cond: call + " == 0", skipped: call + " <= 0",
			src: "package p\nimport \"" + path + "\"\nfunc f(r *" + pkg + "." + typ + ") bool {\n\treturn " + call + " == 0\n}\n",
		})
	}

	for ctor, typ := range constructors {
		path, name := ctor[0], ctor[1]
		pkg := path[strings.LastIndex(path, "/")+1:]
		call := "r." + methodOf(typ) + "()"
		checks = append(checks, check{
			name: path + "." + name, cond: call + " == 0", skipped: call + " <= 0",
			src: "package p\nimport \"" + path + "\"\nfunc f(x int) bool {\n\tr := " + pkg + "." + name + "(x)\n\treturn " + call + " == 0\n}\n",
		})
	}

	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			line := strings.TrimSpace(lineWith(c.src, c.cond))
			if got := comparisonMutantLines(t, c.src); slices.Contains(got, strings.Replace(line, c.cond, c.skipped, 1)) {
				t.Fatalf("%s: the equivalent mutant %s is still generated", c.name, c.skipped)
			}
		})
	}
}

// splitType splits "import/path.Type" into its import path and type name.
func splitType(qualified string) (path, typ string) {
	i := strings.LastIndex(qualified, ".")

	return qualified[:i], qualified[i+1:]
}

// methodOf returns a method of typ in nonNegativeMethods.
func methodOf(typ string) string {
	for method := range nonNegativeMethods {
		if method[0] == typ {
			return method[1]
		}
	}

	return ""
}
