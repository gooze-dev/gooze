package mutagens

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"
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
