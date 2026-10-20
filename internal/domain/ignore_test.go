package domain

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
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

	if !idx.file.ignores(mgen.Arithmetic) {
		t.Fatalf("expected file-level ignore for arithmetic")
	}
	if idx.file.ignores(mgen.Numbers) {
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

			if rule, ok := idx.line[targetLine]; !ok || !rule.ignores(mgen.Arithmetic) {
				t.Fatalf("expected line-level ignore for arithmetic on target line %d", targetLine)
			}

			seenTargets[targetLine] = true
		}
	}

	if len(seenTargets) != 2 {
		t.Fatalf("expected 2 line-level ignore targets, got %d", len(seenTargets))
	}
}

func TestParseIgnoreDirective_RequiresExactKeyword(t *testing.T) {
	for _, text := range []string{"//gooze:ignored", "//gooze:ignore:arithmetic", "//gooze:ignores all"} {
		if _, ok := parseIgnoreDirective(text); ok {
			t.Fatalf("%q must not be treated as a directive", text)
		}
	}
}

func TestParseIgnoreDirective_SpaceSeparatedNames(t *testing.T) {
	r, ok := parseIgnoreDirective("//gooze:ignore arithmetic comparison")
	if !ok || r.all || len(r.names) != 2 {
		t.Fatalf("expected two names, got ok=%v rule=%+v", ok, r)
	}
}

func TestBuildIgnoreIndex_LeadingDirectiveWithFollowingCommentLine(t *testing.T) {
	const src = "package p\n\n" +
		"func f(a int) int {\n" +
		"\t//gooze:ignore arithmetic\n" +
		"\t// explains why the next line is exempt\n" +
		"\t_ = a + 1\n" +
		"\treturn a\n" +
		"}\n"

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "p.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	idx := buildIgnoreIndex(file, fset, []byte(src))

	rule, ok := idx.line[6]
	if !ok || !rule.ignores(mgen.Arithmetic) {
		t.Fatalf("expected line 6 to ignore arithmetic, got %+v (ok=%v)", rule, ok)
	}
}
