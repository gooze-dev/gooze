package domain

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	m "gooze.dev/pkg/gooze/internal/model"
)

func TestParseIgnoreDirective_All(t *testing.T) {
	r, ok := parseIgnoreDirective("//gooze:ignore")
	if !ok {
		t.Fatalf("expected directive to be parsed")
	}
	if !r.all || r.names != nil {
		t.Fatalf("expected all=true and names=nil")
	}
}

func TestParseIgnoreDirective_Names(t *testing.T) {
	r, ok := parseIgnoreDirective("//gooze:ignore Arithmetic, comparison ")
	if !ok {
		t.Fatalf("expected directive to be parsed")
	}
	if r.all {
		t.Fatalf("expected all=false")
	}
	if len(r.names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(r.names))
	}
	if _, ok := r.names["arithmetic"]; !ok {
		t.Fatalf("expected arithmetic")
	}
	if _, ok := r.names["comparison"]; !ok {
		t.Fatalf("expected comparison")
	}
}

func TestParseIgnoreDirective_BlockComment(t *testing.T) {
	r, ok := parseIgnoreDirective("/* gooze:ignore numbers */")
	if !ok {
		t.Fatalf("expected directive to be parsed")
	}
	if r.all {
		t.Fatalf("expected all=false")
	}
	if _, ok := r.names["numbers"]; !ok {
		t.Fatalf("expected numbers")
	}
}

func TestBuildIgnoreIndex_FileFuncLineScopes(t *testing.T) {
	const src = "//gooze:ignore arithmetic\n" +
		"package p\n\n" +
		"//gooze:ignore\n" +
		"func ignoredFunc() {\n" +
		"\t_ = 1 + 2\n" +
		"}\n\n" +
		"func f() {\n" +
		"\t//gooze:ignore arithmetic\n" +
		"\t_ = 1 + 2\n" +
		"\t_ = 1 + 2 //gooze:ignore arithmetic\n" +
		"}\n"

	content := []byte(src)
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", content, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	idx := buildIgnoreIndex(file, fset, content)

	if !idx.file.ignores(m.MutationArithmetic) {
		t.Fatalf("expected file-level ignore for arithmetic")
	}
	if idx.file.ignores(m.MutationNumbers) {
		t.Fatalf("did not expect file-level ignore for numbers")
	}

	if len(idx.funcByPos) == 0 {
		t.Fatalf("expected function ignore rules")
	}

	// Line-level: compute target lines from the comment positions (leading => next line, trailing => same line).
	lineStarts := computeLineStarts(content)
	seenTargets := map[int]bool{}

	for _, group := range file.Comments {
		if group.End() < file.Package {
			continue
		}

		for _, c := range group.List {
			if !strings.Contains(c.Text, "gooze:ignore arithmetic") {
				continue
			}

			pos := fset.PositionFor(c.Slash, true)
			targetLine := pos.Line
			if isLeadingComment(pos.Line, pos.Offset, lineStarts, content) {
				targetLine = pos.Line + 1
			}

			if rule, ok := idx.line[targetLine]; !ok || !rule.ignores(m.MutationArithmetic) {
				t.Fatalf("expected line-level ignore for arithmetic on target line %d", targetLine)
			}

			seenTargets[targetLine] = true
		}
	}

	if len(seenTargets) != 2 {
		t.Fatalf("expected 2 line-level ignore targets, got %d", len(seenTargets))
	}
}
