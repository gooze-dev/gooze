package mutagens

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOffsetForPos(t *testing.T) {
	booleanPath := filepath.Join("..", "..", "..", "examples", "boolean", "main.go")
	content, err := os.ReadFile(booleanPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", booleanPath, err)
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, booleanPath, content, parser.ParseComments)
	if err != nil {
		t.Fatalf("failed to parse source %s: %v", booleanPath, err)
	}

	var literalPos token.Pos
	ast.Inspect(file, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		if ident.Name == "true" || ident.Name == "false" {
			literalPos = ident.Pos()
			return false
		}
		return true
	})

	if literalPos == token.NoPos {
		t.Fatal("expected to find boolean literal")
	}

	offset, ok := offsetForPos(fset, literalPos)
	if !ok {
		t.Fatal("expected offsetForPosV2 to return ok")
	}

	if !(bytes.HasPrefix(content[offset:], []byte("true")) || bytes.HasPrefix(content[offset:], []byte("false"))) {
		start := offset
		end := offset + 10
		if start < 0 {
			start = 0
		}
		if end > len(content) {
			end = len(content)
		}
		snippet := string(content[start:end])
		t.Fatalf("unexpected offset %d; snippet=%q", offset, snippet)
	}
}

func TestReplaceRange(t *testing.T) {
	original := []byte("abcde")
	mutated := replaceRange(original, 1, 4, "XYZ")

	if string(mutated) != "aXYZe" {
		t.Fatalf("replaceRangeV2 result = %q, expected %q", string(mutated), "aXYZe")
	}

	unchanged := replaceRange(original, 10, 12, "nope")
	if !bytes.Equal(unchanged, original) {
		t.Fatalf("expected out-of-range replacement to return original content")
	}
}

func TestEnsureTrailingNewline(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{
			name:     "empty content",
			input:    []byte{},
			expected: []byte{},
		},
		{
			name:     "content with newline",
			input:    []byte("hello world\n"),
			expected: []byte("hello world\n"),
		},
		{
			name:     "content without newline",
			input:    []byte("hello world"),
			expected: []byte("hello world\n"),
		},
		{
			name:     "multiline with newline",
			input:    []byte("line1\nline2\n"),
			expected: []byte("line1\nline2\n"),
		},
		{
			name:     "multiline without newline",
			input:    []byte("line1\nline2"),
			expected: []byte("line1\nline2\n"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ensureTrailingNewline(tt.input)
			if !bytes.Equal(result, tt.expected) {
				t.Errorf("ensureTrailingNewline() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestViableOps_NilAndStringOperands(t *testing.T) {
	parse := func(src string) *ast.BinaryExpr {
		t.Helper()

		expr, err := parser.ParseExpr(src)
		if err != nil {
			t.Fatal(err)
		}

		return expr.(*ast.BinaryExpr)
	}

	alternatives := []token.Token{token.LSS, token.GTR, token.LEQ, token.GEQ, token.EQL}
	if got := viableOps(parse("err != nil"), alternatives); len(got) != 1 || got[0] != token.EQL {
		t.Fatalf("nil comparison alternatives = %v, want [==]", got)
	}

	arith := []token.Token{token.SUB, token.MUL, token.QUO, token.REM}
	if got := viableOps(parse(`name + ".yaml"`), arith); len(got) != 0 {
		t.Fatalf("string concatenation alternatives = %v, want none", got)
	}

	if got := viableOps(parse("a + b"), arith); len(got) != len(arith) {
		t.Fatalf("plain arithmetic alternatives = %v, want all", got)
	}
}

func TestDeleteRange(t *testing.T) {
	cases := []struct {
		name, code, stmt, want string
	}{
		{"alone on its line", "a\n\tx = 1\nb\n", "x = 1", "a\nb\n"},
		{"last line", "a\n\tx = 1", "x = 1", "a\n"},
		{"inside a function literal", "\tdefer func() { x = 1 }()\n", "x = 1", "\tdefer func() {  }()\n"},
		{"before a comment", "\tx = 1 // why\n", "x = 1", "\t // why\n"},
	}

	for _, tc := range cases {
		start := strings.Index(tc.code, tc.stmt)
		got := deleteRange([]byte(tc.code), start, start+len(tc.stmt))
		assert.Equal(t, tc.want, string(got), tc.name)
	}

	assert.Equal(t, "abc", string(deleteRange([]byte("abc"), 2, 1)), "an invalid range changes nothing")
}
