package mutagens

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	m "gooze.dev/pkg/gooze/internal/model"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
	rpt "gooze.dev/pkg/gooze/pkg/report"
)

func TestGenerateComparisonMutations(t *testing.T) {
	tests := []struct {
		name          string
		code          string
		expectedCount int
		expectedType  mgen.Type
	}{
		{
			name:          "less than operator",
			code:          "package main\nfunc test() { x := 5 < 10 }",
			expectedCount: 5, // <, >, <=, >=, ==, != minus original <
			expectedType:  mgen.Comparison,
		},
		{
			name:          "greater than operator",
			code:          "package main\nfunc test() { x := 5 > 10 }",
			expectedCount: 5,
			expectedType:  mgen.Comparison,
		},
		{
			name:          "less than or equal operator",
			code:          "package main\nfunc test() { x := 5 <= 10 }",
			expectedCount: 5,
			expectedType:  mgen.Comparison,
		},
		{
			name:          "greater than or equal operator",
			code:          "package main\nfunc test() { x := 5 >= 10 }",
			expectedCount: 5,
			expectedType:  mgen.Comparison,
		},
		{
			name:          "equal operator",
			code:          "package main\nfunc test() { x := 5 == 10 }",
			expectedCount: 5,
			expectedType:  mgen.Comparison,
		},
		{
			name:          "not equal operator",
			code:          "package main\nfunc test() { x := 5 != 10 }",
			expectedCount: 5,
			expectedType:  mgen.Comparison,
		},
		{
			name:          "non-comparison operator",
			code:          "package main\nfunc test() { x := 5 + 10 }",
			expectedCount: 0,
			expectedType:  mgen.Comparison,
		},
		{
			name:          "multiple comparison operators",
			code:          "package main\nfunc test() { x := 5 < 10 && 20 > 15 }",
			expectedCount: 10, // 5 mutations for each operator
			expectedType:  mgen.Comparison,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "test.go", tt.code, parser.AllErrors)
			if err != nil {
				t.Fatalf("Failed to parse code: %v", err)
			}

			content := []byte(tt.code)
			source := rpt.Source{
				Origin: &rpt.File{FullPath: "test.go"},
			}
			mutations := []m.Mutation{}

			// Traverse AST and collect mutations
			ast.Inspect(file, func(n ast.Node) bool {
				mutations = append(mutations, GenerateComparisonMutations(n, fset, content, source)...)
				return true
			})

			if len(mutations) != tt.expectedCount {
				t.Errorf("Expected %d mutations, got %d", tt.expectedCount, len(mutations))
			}

			for _, mut := range mutations {
				if mut.Type != tt.expectedType {
					t.Errorf("Expected mutation type %v, got %v", tt.expectedType, mut.Type)
				}
			}
		})
	}
}

func TestIsLenOrCapCall(t *testing.T) {
	if isLenOrCapCall(ast.NewIdent("len")) {
		t.Error("an identifier isn't a call")
	}

	if !isLenOrCapCall(&ast.CallExpr{Fun: ast.NewIdent("cap")}) {
		t.Error("cap(x) is a len or cap call")
	}
}

func TestIsComparisonOp(t *testing.T) {
	tests := []struct {
		name     string
		op       token.Token
		expected bool
	}{
		{"less than", token.LSS, true},
		{"greater than", token.GTR, true},
		{"less than or equal", token.LEQ, true},
		{"greater than or equal", token.GEQ, true},
		{"equal", token.EQL, true},
		{"not equal", token.NEQ, true},
		{"addition", token.ADD, false},
		{"subtraction", token.SUB, false},
		{"logical and", token.LAND, false},
		{"logical or", token.LOR, false},
		{"define, above every comparison", token.DEFINE, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isComparisonOp(tt.op)
			if result != tt.expected {
				t.Errorf("isComparisonOp(%v) = %v, expected %v", tt.op, result, tt.expected)
			}
		})
	}
}

func TestGetComparisonAlternatives(t *testing.T) {
	tests := []struct {
		name             string
		original         token.Token
		expectedCount    int
		shouldNotInclude token.Token
	}{
		{"less than", token.LSS, 5, token.LSS},
		{"greater than", token.GTR, 5, token.GTR},
		{"less than or equal", token.LEQ, 5, token.LEQ},
		{"greater than or equal", token.GEQ, 5, token.GEQ},
		{"equal", token.EQL, 5, token.EQL},
		{"not equal", token.NEQ, 5, token.NEQ},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			alternatives := getComparisonAlternatives(tt.original)

			if len(alternatives) != tt.expectedCount {
				t.Errorf("Expected %d alternatives, got %d", tt.expectedCount, len(alternatives))
			}

			for _, alt := range alternatives {
				if alt == tt.shouldNotInclude {
					t.Errorf("Alternatives should not include original operator %v", tt.original)
				}
			}
		})
	}
}

func TestGenerateComparisonMutations_SkipsEquivalentMutantsAgainstTypeBounds(t *testing.T) {
	tests := []struct {
		name    string
		decl    string // of v, inside f; "" uses the parameter v int64
		x, y    string
		op      string
		skipped string // the equivalent mutant; "" keeps all five
	}{
		// Minimum on the right.
		{name: "== min", x: "v", op: "==", y: "math.MinInt64", skipped: "v <= math.MinInt64"},
		{name: "!= min", x: "v", op: "!=", y: "math.MinInt64", skipped: "v > math.MinInt64"},
		{name: "> min", x: "v", op: ">", y: "math.MinInt64", skipped: "v != math.MinInt64"},
		{name: "<= min", x: "v", op: "<=", y: "math.MinInt64", skipped: "v == math.MinInt64"},
		// Minimum on the left.
		{name: "min ==", x: "math.MinInt64", op: "==", y: "v", skipped: "math.MinInt64 >= v"},
		{name: "min <", x: "math.MinInt64", op: "<", y: "v", skipped: "math.MinInt64 != v"},
		// Maximum on the right.
		{name: "== max", x: "v", op: "==", y: "math.MaxInt64", skipped: "v >= math.MaxInt64"},
		{name: "!= max", x: "v", op: "!=", y: "math.MaxInt64", skipped: "v < math.MaxInt64"},
		{name: "< max", x: "v", op: "<", y: "math.MaxInt64", skipped: "v != math.MaxInt64"},
		{name: ">= max", x: "v", op: ">=", y: "math.MaxInt64", skipped: "v == math.MaxInt64"},
		// Maximum on the left.
		{name: "max ==", x: "math.MaxInt64", op: "==", y: "v", skipped: "math.MaxInt64 <= v"},
		{name: "max >", x: "math.MaxInt64", op: ">", y: "v", skipped: "math.MaxInt64 != v"},
		// The other operators have no equivalent mutant.
		{name: "< min", x: "v", op: "<", y: "math.MinInt64"},
		{name: "> max", x: "v", op: ">", y: "math.MaxInt64"},
		// Other integer types.
		{name: "var int8", decl: "var w int8", x: "w", op: "==", y: "math.MinInt8", skipped: "w <= math.MinInt8"},
		{name: "var int", decl: "var w int", x: "w", op: "==", y: "math.MaxInt", skipped: "w >= math.MaxInt"},
		{name: "var uint16", decl: "var w uint16", x: "w", op: "==", y: "math.MaxUint16", skipped: "w >= math.MaxUint16"},
		// A constant inside the variable's range isn't a bound.
		{name: "wider variable, min", x: "v", op: "==", y: "math.MinInt32"},
		{name: "wider variable, max", x: "v", op: "==", y: "math.MaxInt32"},
		{name: "unsigned, max of signed", decl: "var w uint64", x: "w", op: "==", y: "math.MaxInt64"},
		// The variable's type isn't known to be that integer.
		{name: "float", decl: "var w float64", x: "w", op: "==", y: "math.MinInt64"},
		{name: "defined from a call", decl: "w := g()", x: "w", op: "==", y: "math.MinInt64"},
		{name: "a type named int64", decl: "type int64 float64\n\tvar w int64", x: "w", op: "==", y: "math.MinInt64"},
		{name: "an alias, whose bound math names after another type", decl: "var w byte", x: "w", op: "==", y: "math.MaxUint8"},
		{name: "a type math has no such constant for", decl: "var w string", x: "w", op: "==", y: "math.Maxstring"},
		{name: "a call", x: "g()", op: "==", y: "math.MinInt64"},
		// Not math's constant.
		{name: "other package", x: "v", op: "==", y: "other.MinInt64"},
		{name: "package sorting after math", x: "v", op: "==", y: "limits.MinInt64"},
		{name: "local named math", decl: "math := s{}", x: "v", op: "==", y: "math.MinInt64"},
		{name: "not a selector", x: "v", op: "==", y: "limit"},
		{name: "method of a call", x: "v", op: "==", y: "g().MinInt64"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr := tt.x + " " + tt.op + " " + tt.y
			src := "package p\n\nimport (\n\t\"math\"\n\t\"example.com/other\"\n\t\"sync/limits\"\n)\n\nfunc f(v int64) bool {\n\t" +
				tt.decl + "\n\treturn " + expr + "\n}\n"

			fset := token.NewFileSet()

			file, err := parser.ParseFile(fset, "p.go", src, 0)
			if err != nil {
				t.Fatal(err)
			}

			var mutations []m.Mutation

			ast.Inspect(file, func(n ast.Node) bool {
				mutations = append(mutations, GenerateComparisonMutations(n, fset, []byte(src), rpt.Source{})...)
				return true
			})

			want := 5
			if tt.skipped != "" {
				want = 4
			}

			if len(mutations) != want {
				t.Fatalf("%s: got %d mutants, want %d", expr, len(mutations), want)
			}

			for _, mutation := range mutations {
				if tt.skipped != "" && strings.Contains(string(mutation.MutatedCode), "return "+tt.skipped+"\n") {
					t.Fatalf("%s: equivalent mutant %q was generated", expr, tt.skipped)
				}
			}
		})
	}
}

// rangeKeyMutants returns the conditions the comparison mutants of body turn
// its if statements into, sorted. The ranges over the expressions named in
// indexed get Facts that say so, as type information would.
func rangeKeyMutants(t *testing.T, body string, indexed ...string) []string {
	t.Helper()

	code := "package main\n\nfunc f(xs []int, m map[int]int, ys []int) {\n" + body + "\n}\n"
	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, "test.go", code, parser.AllErrors)
	require.NoError(t, err)

	facts := Facts{Indexed: map[[2]int]bool{}}

	ast.Inspect(file, func(n ast.Node) bool {
		if r, ok := n.(*ast.RangeStmt); ok {
			start, end := fset.Position(r.X.Pos()).Offset, fset.Position(r.X.End()).Offset
			if slices.Contains(indexed, code[start:end]) {
				facts.Indexed[[2]int{start, end}] = true
			}
		}

		return true
	})

	var conditions []string

	ast.Inspect(file, func(n ast.Node) bool {
		for _, mutation := range GenerateComparisonMutationsWith(n, fset, []byte(code), rpt.Source{}, facts) {
			for line := range strings.Lines(string(mutation.MutatedCode)) {
				if cond, ok := strings.CutPrefix(strings.TrimSpace(line), "if "); ok && !strings.Contains(code, line) {
					conditions = append(conditions, strings.TrimSuffix(cond, " {}"))
				}
			}
		}

		return true
	})

	slices.Sort(conditions)

	return conditions
}

func TestGenerateComparisonMutations_RangeKeys(t *testing.T) {
	cases := map[string]struct {
		body    string
		indexed []string
		has     []string // mutants that must be generated
		hasNot  []string // equivalent mutants that must not be
	}{
		"key of a slice is never negative": {
			body: "for i := range xs {\nif i == 0 {}\n}", indexed: []string{"xs"},
			has: []string{"i != 0", "i < 0", "i > 0", "i >= 0"}, hasNot: []string{"i <= 0"},
		},
		"key with a value": {
			body: "for i, x := range xs {\nif i > 0 {}\n_ = x\n}", indexed: []string{"xs"},
			has: []string{"i < 0", "i <= 0", "i == 0", "i >= 0"}, hasNot: []string{"i != 0"},
		},
		"key of an integer": {
			body: "for i := range 10 {\nif i == 0 {}\n}", indexed: []string{"10"},
			hasNot: []string{"i <= 0"},
		},
		"without type information": {
			body: "for i := range xs {\nif i == 0 {}\n}",
			has:  []string{"i <= 0"},
		},
		"key of a map can be negative": {
			body: "for k := range m {\nif k == 0 {}\n}", indexed: []string{"xs"},
			has: []string{"k <= 0"},
		},
		"the value is not the key": {
			body: "for _, v := range xs {\nif v == 0 {}\n}", indexed: []string{"xs"},
			has: []string{"v <= 0"},
		},
		"a key the loop writes": {
			body: "for i := range xs {\ni--\nif i == 0 {}\n}", indexed: []string{"xs"},
			has: []string{"i <= 0"},
		},
		"key below len minus one": {
			body: "for i := range xs {\nif i < len(xs)-1 {}\n}", indexed: []string{"xs"},
			has: []string{"i <= len(xs)-1", "i == len(xs)-1", "i > len(xs)-1", "i >= len(xs)-1"}, hasNot: []string{"i != len(xs)-1"},
		},
		"key at len minus one": {
			body: "for i := range xs {\nif i == len(xs)-1 {}\n}", indexed: []string{"xs"},
			hasNot: []string{"i >= len(xs)-1"},
		},
		"len minus one on the left": {
			body: "for i := range xs {\nif len(xs)-1 > i {}\n}", indexed: []string{"xs"},
			hasNot: []string{"len(xs)-1 != i"},
		},
		"cap minus one can be past the last key": {
			body: "for i := range xs {\nif i < cap(xs)-1 {}\n}", indexed: []string{"xs"},
			has: []string{"i != cap(xs)-1"},
		},
		"len of another slice": {
			body: "for i := range xs {\nif i < len(ys)-1 {}\n}", indexed: []string{"xs", "ys"},
			has: []string{"i != len(ys)-1"},
		},
		"a slice the loop grows": {
			body: "for i := range xs {\nxs = append(xs, 1)\nif i < len(xs)-1 {}\n}", indexed: []string{"xs"},
			has: []string{"i != len(xs)-1"},
		},
		"a value named before the key": {
			body: "for i, a := range xs {\nif a == 0 {}\n_ = i\n}", indexed: []string{"xs"},
			has: []string{"a <= 0"},
		},
		"a key a later range assigns": {
			body: "for i := range xs {\nfor i = range ys {}\nif i == 0 {}\n}", indexed: []string{"xs", "ys"},
			has: []string{"i <= 0"},
		},
		"a key a later range assigns as its value": {
			body: "for i := range xs {\nfor _, i = range ys {}\nif i == 0 {}\n}", indexed: []string{"xs", "ys"},
			has: []string{"i <= 0"},
		},
		"len bound of a range over a literal": {
			body: "for i := range 10 {\nif i < len(xs)-1 {}\n}", indexed: []string{"10"},
			has: []string{"i != len(xs)-1"},
		},
		"len bound of a package-level slice": {
			body: "for i := range other {\nif i < len(other)-1 {}\n}", indexed: []string{"other"},
			has: []string{"i != len(other)-1"},
		},
		"len plus one": {
			body: "for i := range xs {\nif i < len(xs)+1 {}\n}", indexed: []string{"xs"},
			has: []string{"i != len(xs)+1"},
		},
		"len divided": {
			body: "for i := range xs {\nif i < len(xs)/1 {}\n}", indexed: []string{"xs"},
			has: []string{"i != len(xs)/1"},
		},
		"len minus two": {
			body: "for i := range xs {\nif i < len(xs)-2 {}\n}", indexed: []string{"xs"},
			has: []string{"i != len(xs)-2"},
		},
		"len itself": {
			body: "for i := range xs {\nif i < len(xs) {}\n}", indexed: []string{"xs"},
			has: []string{"i != len(xs)"},
		},
		"another builtin": {
			body: "for i := range xs {\nif i < min(xs)-1 {}\n}", indexed: []string{"xs"},
			has: []string{"i != min(xs)-1"},
		},
		"nested keys": {
			body: "for i := range xs {\nfor j := range ys {\nif i == 0 && j == 0 {}\n}\n}", indexed: []string{"xs", "ys"},
			hasNot: []string{"i <= 0 && j == 0", "i == 0 && j <= 0"},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := rangeKeyMutants(t, tc.body, tc.indexed...)

			for _, want := range tc.has {
				assert.Contains(t, got, want)
			}

			for _, unwanted := range tc.hasNot {
				assert.NotContains(t, got, unwanted)
			}
		})
	}
}
