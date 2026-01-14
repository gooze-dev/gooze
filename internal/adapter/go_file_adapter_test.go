package adapter

import (
	"reflect"
	"testing"

	"go/token"

	m "github.com/mouse-blink/gooze/internal/model"
)

const sampleSource = `package sample

const flag = true
var number = 1

func init() {
}

func add(a int) int {
    return a + 1
}
`

func TestLocalGoFileAdapter_Parse(t *testing.T) {
	adapter := NewLocalGoFileAdapter()
	fset := token.NewFileSet()

	file, err := adapter.Parse(fset, "sample.go", []byte(sampleSource))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if file.Name.Name != "sample" {
		t.Fatalf("Parse() package = %s, want sample", file.Name.Name)
	}
}

func TestLocalGoFileAdapter_Parse_InvalidSource(t *testing.T) {
	adapter := NewLocalGoFileAdapter()
	fset := token.NewFileSet()

	if _, err := adapter.Parse(fset, "broken.go", []byte("package foo\n func")); err == nil {
		t.Fatalf("Parse() expected error for invalid source")
	}
}

func TestLocalGoFileAdapter_ExtractScopes(t *testing.T) {
	adapter := NewLocalGoFileAdapter()
	fset := token.NewFileSet()
	file, err := adapter.Parse(fset, "sample.go", []byte(sampleSource))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	scopes := adapter.ExtractScopes(fset, file)
	if len(scopes) == 0 {
		t.Fatalf("ExtractScopes() returned no scopes")
	}

	counts := map[m.ScopeType]int{}
	for _, scope := range scopes {
		counts[scope.Type]++
	}

	if counts[m.ScopeGlobal] != 2 {
		t.Fatalf("expected 2 global scopes, got %d", counts[m.ScopeGlobal])
	}
	if counts[m.ScopeInit] != 1 {
		t.Fatalf("expected 1 init scope, got %d", counts[m.ScopeInit])
	}
	if counts[m.ScopeFunction] != 1 {
		t.Fatalf("expected 1 function scope, got %d", counts[m.ScopeFunction])
	}
}

func TestLocalGoFileAdapter_ExtractScopes_NoMatches(t *testing.T) {
	adapter := NewLocalGoFileAdapter()
	fset := token.NewFileSet()
	file, err := adapter.Parse(fset, "empty.go", []byte("package empty\n"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	scopes := adapter.ExtractScopes(fset, file)
	if len(scopes) != 0 {
		t.Fatalf("ExtractScopes() = %d, want 0", len(scopes))
	}
}

func TestLocalGoFileAdapter_FunctionLines(t *testing.T) {
	adapter := NewLocalGoFileAdapter()
	fset := token.NewFileSet()
	file, err := adapter.Parse(fset, "sample.go", []byte(sampleSource))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	scopes := adapter.ExtractScopes(fset, file)
	lines := adapter.FunctionLines(scopes)
	want := []int{6, 9}
	if !reflect.DeepEqual(lines, want) {
		t.Fatalf("FunctionLines() = %v, want %v", lines, want)
	}
}

func TestLocalGoFileAdapter_FunctionLines_Empty(t *testing.T) {
	adapter := NewLocalGoFileAdapter()
	scopes := []m.CodeScope{{Type: m.ScopeGlobal, StartLine: 1, EndLine: 1}}

	if lines := adapter.FunctionLines(scopes); len(lines) != 0 {
		t.Fatalf("FunctionLines() = %v, want []", lines)
	}
}

func TestLocalGoFileAdapter_ScopeForLine(t *testing.T) {
	adapter := NewLocalGoFileAdapter()
	fset := token.NewFileSet()
	file, err := adapter.Parse(fset, "sample.go", []byte(sampleSource))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	scopes := adapter.ExtractScopes(fset, file)

	tests := []struct {
		line int
		want m.ScopeType
	}{
		{line: 3, want: m.ScopeGlobal},
		{line: 6, want: m.ScopeInit},
		{line: 9, want: m.ScopeFunction},
		{line: 20, want: m.ScopeFunction}, // default fallback
	}

	for _, tt := range tests {
		if got := adapter.ScopeForLine(scopes, tt.line); got != tt.want {
			t.Fatalf("ScopeForLine(%d) = %s, want %s", tt.line, got, tt.want)
		}
	}
}

func TestLocalGoFileAdapter_ScopeForLine_NoScopes(t *testing.T) {
	adapter := NewLocalGoFileAdapter()

	if got := adapter.ScopeForLine(nil, 10); got != m.ScopeFunction {
		t.Fatalf("ScopeForLine() = %s, want %s", got, m.ScopeFunction)
	}
}
