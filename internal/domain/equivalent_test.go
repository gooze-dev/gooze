package domain_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	m "gooze.dev/pkg/gooze/internal/model"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
)

// mutatedLines returns the lines of src that mutationType mutates, in order.
func mutatedLines(t *testing.T, src string, mutationType mgen.Type) []int {
	t.Helper()

	return linesOf(streamTypesFromSource(t, src, mutationType))
}

func TestMutagen_SkipsOtherResultsOfErrorAndCommaOkReturns(t *testing.T) {
	tests := []struct {
		name         string
		src          string
		mutationType mgen.Type
		want         []int // the mutated lines
	}{
		{
			name: "values returned with an error", mutationType: mgen.Numbers,
			src: "package p\n\nimport \"errors\"\n\nfunc f() (int, int, error) {\n\treturn 0, 1, errors.New(\"x\")\n}\n",
		},
		{
			name: "the error expression is still mutated", mutationType: mgen.Numbers, want: []int{6, 6},
			src: "package p\n\nimport \"fmt\"\n\nfunc f() (int, error) {\n\treturn 0, fmt.Errorf(\"bad %d\", 5)\n}\n",
		},
		{
			name: "values returned with an error variable", mutationType: mgen.Numbers,
			src: "package p\n\nfunc f(err error) (int, error) {\n\treturn 0, err\n}\n",
		},
		{
			name: "a boolean returned with an error", mutationType: mgen.Boolean,
			src: "package p\n\nfunc f(err error) (bool, error) {\n\treturn false, err\n}\n",
		},
		{
			name: "values returned with a false ok", mutationType: mgen.Numbers,
			src: "package p\n\nfunc f() (string, int, bool) {\n\treturn \"\", 0, false\n}\n",
		},
		{
			name: "negative and composite zero values", mutationType: mgen.Numbers,
			src: "package p\n\ntype T struct{ A, B int }\n\nfunc f(err error) (int, T, error) {\n\treturn -1, T{A: 1, B: 2}, err\n}\n",
		},
		{
			name: "function literal", mutationType: mgen.Numbers,
			src: "package p\n\nfunc f() func() (int, error) {\n\treturn func() (int, error) {\n\t\treturn 7, errNope\n\t}\n}\n\nvar errNope error\n",
		},
		{
			name: "values returned with a nil error", mutationType: mgen.Numbers, want: []int{4},
			src: "package p\n\nfunc f() (int, error) {\n\treturn 1, nil\n}\n",
		},
		{
			name: "values returned with a true ok", mutationType: mgen.Numbers, want: []int{4},
			src: "package p\n\nfunc f() (int, bool) {\n\treturn 1, true\n}\n",
		},
		{
			name: "ok that isn't a literal", mutationType: mgen.Numbers, want: []int{4},
			src: "package p\n\nfunc f(ok bool) (int, bool) {\n\treturn 0, ok\n}\n",
		},
		{
			name: "computed value returned with an error", mutationType: mgen.Arithmetic, want: []int{4, 4, 4, 4},
			src: "package p\n\nfunc f(n int, err error) (int, error) {\n\treturn n + 1, err\n}\n",
		},
		{
			name: "last result is not an error or bool", mutationType: mgen.Numbers, want: []int{4, 4, 4},
			src: "package p\n\nfunc f() (int, int) {\n\treturn 1, 2\n}\n",
		},
		{
			name: "a type named error", mutationType: mgen.Numbers, want: []int{6},
			src: "package p\n\ntype error int\n\nfunc f(e error) (int, error) {\n\treturn 0, e\n}\n",
		},
		{
			name: "a type named bool", mutationType: mgen.Numbers, want: []int{6, 9},
			src: "package p\n\ntype bool int\n\nfunc f() (int, bool) {\n\treturn 0, false\n}\n\nconst false bool = 0\n",
		},
		{
			name: "a function literal inside a skipped value", mutationType: mgen.Numbers, want: []int{5},
			src: "package p\n\nfunc f(err error) (func() int, error) {\n\treturn func() int {\n\t\treturn 1\n\t}, err\n}\n",
		},
		{
			name: "named results", mutationType: mgen.Numbers,
			src: "package p\n\nimport \"errors\"\n\nfunc f() (a, b int, err error) {\n\treturn 1, 2, errors.New(\"x\")\n}\n",
		},
		{
			name: "address of a composite literal", mutationType: mgen.Numbers,
			src: "package p\n\ntype T struct{ A int }\n\nfunc f(err error) (*T, error) {\n\treturn &T{A: 1}, err\n}\n",
		},
		{
			name: "parenthesized negative literal", mutationType: mgen.Numbers,
			src: "package p\n\nfunc f(err error) (int, error) {\n\treturn -(1), err\n}\n",
		},
		{
			name: "the false ok itself is still mutated", mutationType: mgen.Boolean, want: []int{4},
			src: "package p\n\nfunc f() (string, bool) {\n\treturn \"\", false\n}\n",
		},
		{
			name: "complemented literal", mutationType: mgen.Numbers, want: []int{4},
			src: "package p\n\nfunc f(err error) (int, error) {\n\treturn ^1, err\n}\n",
		},
		{
			name: "composite literal with a computed field", mutationType: mgen.Arithmetic, want: []int{6, 6, 6, 6},
			src: "package p\n\ntype T struct{ A int }\n\nfunc f(n int, err error) (T, error) {\n\treturn T{A: n + 1}, err\n}\n",
		},
		{
			name: "bare return with named results", mutationType: mgen.Numbers, want: []int{4},
			src: "package p\n\nfunc f() (n int, err error) {\n\tn = 1\n\treturn\n}\n",
		},
		{
			name: "function literal with other results inside", mutationType: mgen.Numbers, want: []int{4, 4, 4},
			src: "package p\n\nfunc f(err error) (int, error) {\n\tg := func() (int, int) { return 1, 2 }\n\t_ = g\n\treturn 0, err\n}\n",
		},
		{
			name: "function without results", mutationType: mgen.Numbers, want: []int{5},
			src: "package p\n\nfunc f(n *int) {\n\tif n == nil {\n\t\t*n = 1\n\t\treturn\n\t}\n}\n",
		},
		{
			name: "named ok", mutationType: mgen.Numbers,
			src: "package p\n\nfunc f() (v int, ok bool) {\n\treturn 0, false\n}\n",
		},
		{
			name: "named bool that isn't ok", mutationType: mgen.Boolean, want: []int{4, 4},
			src: "package p\n\nfunc f() (covered, known bool) {\n\treturn false, false\n}\n",
		},
		{
			name: "bool named before ok", mutationType: mgen.Numbers, want: []int{4},
			src: "package p\n\nfunc f() (v int, known bool) {\n\treturn 0, false\n}\n",
		},
		{
			name: "bool named after ok", mutationType: mgen.Numbers, want: []int{4},
			src: "package p\n\nfunc f() (v int, valid bool) {\n\treturn 0, false\n}\n",
		},
		{
			name: "a defined bool type", mutationType: mgen.Numbers, want: []int{6},
			src: "package p\n\ntype flag bool\n\nfunc f() (int, flag) {\n\treturn 0, false\n}\n",
		},
		{
			name: "two unnamed bools", mutationType: mgen.Boolean, want: []int{4, 4},
			src: "package p\n\nfunc f() (bool, bool) {\n\treturn false, false\n}\n",
		},
		{
			name: "a bool next to the ok", mutationType: mgen.Boolean, want: []int{4, 4},
			src: "package p\n\nfunc f() (string, bool, bool) {\n\treturn \"\", true, false\n}\n",
		},
		{
			name: "one call returning everything", mutationType: mgen.Numbers, want: []int{4, 4, 7},
			src: "package p\n\nfunc f() (int, error) {\n\treturn g(3)\n}\n\nfunc g(int) (int, error) { return 1, nil }\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mutatedLines(t, tt.src, tt.mutationType)
			if len(tt.want) == 0 {
				require.Empty(t, got)
				return
			}

			require.Equal(t, tt.want, got)
		})
	}
}

func TestMutagen_SkipsMapSizeHints(t *testing.T) {
	tests := []struct {
		name         string
		src          string
		mutationType mgen.Type
		want         []int // the mutated lines
	}{
		{
			name: "map size hint", mutationType: mgen.Arithmetic,
			src: "package p\n\nfunc f(n int) map[string]int {\n\treturn make(map[string]int, n*2)\n}\n",
		},
		{
			name: "map size hint literal", mutationType: mgen.Numbers,
			src: "package p\n\nfunc f() map[string]int {\n\treturn make(map[string]int, 8)\n}\n",
		},
		{
			name: "map size hint dividing by a variable", mutationType: mgen.Arithmetic, want: []int{4, 4, 4, 4},
			src: "package p\n\nfunc f(n, k int) map[string]int {\n\treturn make(map[string]int, n+k)\n}\n",
		},
		{
			name: "map without a size hint", mutationType: mgen.Arithmetic, want: []int{5, 5, 5, 5},
			src: "package p\n\nfunc f(n int) (map[string]int, int) {\n\tm := make(map[string]int)\n\treturn m, n + 1\n}\n",
		},
		{
			name: "slice capacity, which can panic below the length", mutationType: mgen.Arithmetic, want: []int{4, 4, 4, 4},
			src: "package p\n\nfunc f(n int) []int {\n\treturn make([]int, 0, n+1)\n}\n",
		},
		{
			name: "channel buffer size", mutationType: mgen.Arithmetic, want: []int{4, 4, 4, 4},
			src: "package p\n\nfunc f(n int) chan int {\n\treturn make(chan int, n+1)\n}\n",
		},
		{
			name: "named map type", mutationType: mgen.Arithmetic, want: []int{6, 6, 6, 6},
			src: "package p\n\ntype index map[string]int\n\nfunc f(n int) index {\n\treturn make(index, n+1)\n}\n",
		},
		{
			name: "a function named make", mutationType: mgen.Arithmetic, want: []int{6, 6, 6, 6},
			src: "package p\n\nfunc make(m map[string]int, n int) map[string]int { return m }\n\nfunc f(n int) map[string]int {\n\treturn make(nil, n*2)\n}\n",
		},
		{
			name: "a function literal in the hint", mutationType: mgen.Numbers, want: []int{4},
			src: "package p\n\nfunc f() map[string]int {\n\treturn make(map[string]int, func() int { return 1 }())\n}\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mutatedLines(t, tt.src, tt.mutationType)
			if len(tt.want) == 0 {
				require.Empty(t, got)
				return
			}

			require.Equal(t, tt.want, got)
		})
	}
}

// mutatedLineTexts returns the changed line of each mutation, trimmed and
// sorted.
func mutatedLineTexts(mutations []m.Mutation) []string {
	var texts []string

	for _, mutation := range mutations {
		lines := strings.Split(string(mutation.MutatedCode), "\n")
		texts = append(texts, strings.TrimSpace(lines[mutation.Line-1]))
	}

	slices.Sort(texts)

	return texts
}

func TestMutagen_PerOperatorIgnore(t *testing.T) {
	tests := []struct {
		name         string
		src          string
		mutationType mgen.Type
		want         []string // the changed lines, sorted
	}{
		{
			name: "line ignore of one comparison", mutationType: mgen.Comparison,
			src:  "package p\n\nfunc f(a, b int) bool {\n\treturn a < b //gooze:ignore comparison(!=)\n}\n",
			want: []string{"return a <= b //gooze:ignore comparison(!=)", "return a == b //gooze:ignore comparison(!=)", "return a > b //gooze:ignore comparison(!=)", "return a >= b //gooze:ignore comparison(!=)"},
		},
		{
			name: "leading line ignore of two comparisons", mutationType: mgen.Comparison,
			src:  "package p\n\nfunc f(a, b int) bool {\n\t//gooze:ignore comparison(!=, ==)\n\treturn a < b\n}\n",
			want: []string{"return a <= b", "return a > b", "return a >= b"},
		},
		{
			name: "function ignore of one arithmetic operator", mutationType: mgen.Arithmetic,
			src:  "package p\n\n//gooze:ignore arithmetic(-)\nfunc f(a, b int) int {\n\treturn a + b\n}\n\nfunc g(a, b int) int {\n\treturn a + b\n}\n",
			want: []string{"return a % b", "return a % b", "return a * b", "return a * b", "return a - b", "return a / b", "return a / b"},
		},
		{
			name: "one-line function", mutationType: mgen.Arithmetic,
			src:  "package p\n\n//gooze:ignore arithmetic(-)\nfunc f(a, b int) int { return a + b }\n",
			want: []string{"func f(a, b int) int { return a % b }", "func f(a, b int) int { return a * b }", "func f(a, b int) int { return a / b }"},
		},
		{
			name: "function after a //line comment", mutationType: mgen.Arithmetic,
			src:  "package p\n\n//line other.go:1\n//gooze:ignore arithmetic(-)\nfunc f(a, b int) int {\n\treturn a + b\n}\n",
			want: []string{"return a % b", "return a * b", "return a / b"},
		},
		{
			name: "function holding a //line comment", mutationType: mgen.Arithmetic,
			src:  "package p\n\n//gooze:ignore arithmetic(-)\nfunc f(a, b int) int {\n//line other.go:100\n\treturn a + b\n}\n",
			want: []string{"return a % b", "return a * b", "return a / b"},
		},
		{
			name: "file ignore of one arithmetic operator", mutationType: mgen.Arithmetic,
			src:  "//gooze:ignore arithmetic(*, /)\npackage p\n\nfunc f(a, b int) int {\n\treturn a + b\n}\n",
			want: []string{"return a % b", "return a - b"},
		},
		{
			name: "numbers value", mutationType: mgen.Numbers,
			src:  "package p\n\nfunc f() int {\n\treturn 5 //gooze:ignore numbers(0)\n}\n",
			want: []string{"return 1 //gooze:ignore numbers(0)"},
		},
		{
			name: "boolean value", mutationType: mgen.Boolean,
			src: "package p\n\nfunc f() bool {\n\treturn true //gooze:ignore boolean(false)\n}\n",
		},
		{
			name: "increment", mutationType: mgen.Increment,
			src: "package p\n\nfunc f(c int) int {\n\tc++ //gooze:ignore increment(--)\n\treturn c\n}\n",
		},
		{
			name: "two-character operator replaced by one", mutationType: mgen.Comparison,
			src:  "package p\n\nfunc f(a, b int) bool {\n\treturn a <= b //gooze:ignore comparison(<)\n}\n",
			want: []string{"return a != b //gooze:ignore comparison(<)", "return a == b //gooze:ignore comparison(<)", "return a > b //gooze:ignore comparison(<)", "return a >= b //gooze:ignore comparison(<)"},
		},
		{
			name: "an unknown operator ignores nothing", mutationType: mgen.Comparison,
			src:  "package p\n\nfunc f(a, b int) bool {\n\treturn a < b //gooze:ignore comparison(?)\n}\n",
			want: []string{"return a != b //gooze:ignore comparison(?)", "return a <= b //gooze:ignore comparison(?)", "return a == b //gooze:ignore comparison(?)", "return a > b //gooze:ignore comparison(?)", "return a >= b //gooze:ignore comparison(?)"},
		},
		{
			name: "another mutagen's list ignores nothing", mutationType: mgen.Comparison,
			src:  "package p\n\nfunc f(a, b int) bool {\n\treturn a < b //gooze:ignore arithmetic(!=)\n}\n",
			want: []string{"return a != b //gooze:ignore arithmetic(!=)", "return a <= b //gooze:ignore arithmetic(!=)", "return a == b //gooze:ignore arithmetic(!=)", "return a > b //gooze:ignore arithmetic(!=)", "return a >= b //gooze:ignore arithmetic(!=)"},
		},
		{
			name: "a whole-mutagen ignore next to a list", mutationType: mgen.Comparison,
			src: "package p\n\nfunc f(a, b int) bool {\n\treturn a < b //gooze:ignore comparison(!=), comparison\n}\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mutatedLineTexts(streamTypesFromSource(t, tt.src, tt.mutationType))
			require.Equal(t, tt.want, got)
		})
	}
}
