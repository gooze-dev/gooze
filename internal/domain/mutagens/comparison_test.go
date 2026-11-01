package mutagens

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

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
