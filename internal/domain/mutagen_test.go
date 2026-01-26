package domain

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mouse-blink/gooze/internal/adapter"
	m "github.com/mouse-blink/gooze/internal/model"
)

func TestMutagen_GenerateMutation_ArithmeticBasic(t *testing.T) {
	mg := newTestMutagen()

	source := makeSourceV2(t, filepath.Join("..", "..", "examples", "basic", "main.go"))
	original := readFileBytes(t, source.Origin.FullPath)

	mutations, err := mg.GenerateMutation(source, m.MutationArithmetic)
	if err != nil {
		t.Fatalf("GenerateMutation failed: %v", err)
	}

	if len(mutations) != 4 {
		t.Fatalf("expected 4 mutations for +, got %d", len(mutations))
	}

	expectedOps := map[string]bool{"-": false, "*": false, "/": false, "%": false}

	for i, mutation := range mutations {
		if len(mutation.ID) == 0 {
			t.Fatalf("expected non-empty mutation ID for mutation %d", i)
		}
		if mutation.Type != m.MutationArithmetic {
			t.Fatalf("expected arithmetic mutation, got %v", mutation.Type)
		}
		if bytes.Equal(mutation.MutatedCode, original) {
			t.Fatalf("expected mutated code to differ from original")
		}

		code := string(mutation.MutatedCode)
		for op := range expectedOps {
			if strings.Contains(code, "3"+op+"5") {
				expectedOps[op] = true
			}
		}
	}

	for op, found := range expectedOps {
		if !found {
			t.Errorf("expected mutation to %s, but not found", op)
		}
	}
}

func TestMutagen_GenerateMutation_BooleanLiterals(t *testing.T) {
	mg := newTestMutagen()

	source := makeSourceV2(t, filepath.Join("..", "..", "examples", "boolean", "main.go"))
	original := readFileBytes(t, source.Origin.FullPath)

	mutations, err := mg.GenerateMutation(source, m.MutationBoolean)
	if err != nil {
		t.Fatalf("GenerateMutation failed: %v", err)
	}

	if len(mutations) != 4 {
		t.Fatalf("expected 4 boolean mutations, got %d", len(mutations))
	}

	expectedFragments := map[string]bool{
		"isValid := false":          false,
		"isComplete := true":        false,
		"checkStatus(false, false)": false,
		"checkStatus(true, true)":   false,
	}

	for i, mutation := range mutations {
		if len(mutation.ID) == 0 {
			t.Fatalf("expected non-empty mutation ID for mutation %d", i)
		}
		if mutation.Type != m.MutationBoolean {
			t.Fatalf("expected boolean mutation, got %v", mutation.Type)
		}
		if bytes.Equal(mutation.MutatedCode, original) {
			t.Fatalf("expected mutated code to differ from original")
		}

		code := string(mutation.MutatedCode)
		for fragment := range expectedFragments {
			if strings.Contains(code, fragment) {
				expectedFragments[fragment] = true
			}
		}
	}

	for fragment, found := range expectedFragments {
		if !found {
			t.Errorf("expected mutated code to contain %q", fragment)
		}
	}
}

func TestMutagen_GenerateMutation_DefaultTypes(t *testing.T) {
	mg := newTestMutagen()

	source := makeSourceV2(t, filepath.Join("..", "..", "examples", "basic", "main.go"))
	mutations, err := mg.GenerateMutation(source)
	if err != nil {
		t.Fatalf("GenerateMutation failed: %v", err)
	}

	if len(mutations) != 12 {
		t.Fatalf("expected 12 mutations, got %d", len(mutations))
	}
}

func TestMutagen_GenerateMutation_InvalidType(t *testing.T) {
	mg := newTestMutagen()

	source := makeSourceV2(t, filepath.Join("..", "..", "examples", "basic", "main.go"))
	_, err := mg.GenerateMutation(source, m.MutationType{Name: "invalid", Version: 1})
	if err == nil {
		t.Fatalf("expected error for invalid mutation type")
	}
}

func TestMutagen_GenerateMutation_InvalidSource(t *testing.T) {
	mg := newTestMutagen()

	_, err := mg.GenerateMutation(m.Source{}, m.MutationArithmetic)
	if err == nil {
		t.Fatalf("expected error for missing source origin")
	}
}

func makeSourceV2(t *testing.T, path string) m.Source {
	t.Helper()

	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("failed to resolve path: %v", err)
	}

	return m.Source{
		Origin: &m.File{FullPath: m.Path(abs)},
	}
}

func readFileBytes(t *testing.T, path m.Path) []byte {
	t.Helper()

	content, err := os.ReadFile(string(path))
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}

	return content
}

func newTestMutagen() Mutagen {
	return NewMutagen(adapter.NewLocalGoFileAdapter(), adapter.NewLocalSourceFSAdapter())
}
