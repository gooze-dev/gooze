package adapter

import (
	"context"
	"path/filepath"
	"testing"

	"go/token"
)

func TestLocalGoFileAdapter_Parse(t *testing.T) {
	adapter := NewLocalGoFileAdapter()
	fset := token.NewFileSet()

	exampleFile := filepath.Join(examplePath(t, "basic"), "main.go")
	content := readFileBytes(t, exampleFile)
	file, err := adapter.Parse(context.Background(), fset, exampleFile, content)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if file.Name.Name != "main" {
		t.Fatalf("Parse() package = %s, want main", file.Name.Name)
	}
}

func TestLocalGoFileAdapter_Parse_InvalidSource(t *testing.T) {
	adapter := NewLocalGoFileAdapter()
	fset := token.NewFileSet()

	if _, err := adapter.Parse(context.Background(), fset, "broken.go", []byte("package foo\n func")); err == nil {
		t.Fatalf("Parse() expected error for invalid source")
	}
}

func TestLocalGoFileAdapter_Parse_ContextCancellation(t *testing.T) {
	adapter := NewLocalGoFileAdapter()
	fset := token.NewFileSet()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	if _, err := adapter.Parse(ctx, fset, "example.go", []byte("package main\n func main() {}")); err == nil {
		t.Fatalf("Parse() expected error due to context cancellation")
	}
}
