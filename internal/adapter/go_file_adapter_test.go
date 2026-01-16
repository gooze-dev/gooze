package adapter

import (
	"testing"

	"go/token"
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
