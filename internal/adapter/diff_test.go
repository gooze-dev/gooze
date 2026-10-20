package adapter

import (
	"bytes"
	"testing"
)

func TestUnifiedDiffer(t *testing.T) {
	original := []byte("package main\n\nfunc foo() {\n\treturn 3 + 5\n}")
	mutated := []byte("package main\n\nfunc foo() {\n\treturn 3 - 5\n}")

	diff := NewUnifiedDiffer().Diff(original, mutated)
	diffStr := string(diff)

	if !bytes.Contains(diff, []byte("--- original")) {
		t.Errorf("expected diff to contain '--- original', got: %s", diffStr)
	}
	if !bytes.Contains(diff, []byte("+++ mutated")) {
		t.Errorf("expected diff to contain '+++ mutated', got: %s", diffStr)
	}
	if !bytes.Contains(diff, []byte("-\treturn 3 + 5")) {
		t.Errorf("expected diff to contain removed line, got: %s", diffStr)
	}
	if !bytes.Contains(diff, []byte("+\treturn 3 - 5")) {
		t.Errorf("expected diff to contain added line, got: %s", diffStr)
	}
}

func TestUnifiedDiffer_EmptyInputs(t *testing.T) {
	// Test with both empty
	diff := NewUnifiedDiffer().Diff(nil, nil)
	if diff != nil {
		t.Errorf("expected nil diff for empty inputs, got: %v", diff)
	}

	diff = NewUnifiedDiffer().Diff([]byte{}, []byte{})
	if diff != nil {
		t.Errorf("expected nil diff for empty byte slices, got: %v", diff)
	}

	// Test with one empty
	original := []byte("some content")
	diff = NewUnifiedDiffer().Diff(original, nil)
	if diff == nil {
		t.Errorf("expected diff when one input is empty")
	}

	diff = NewUnifiedDiffer().Diff(nil, original)
	if diff == nil {
		t.Errorf("expected diff when one input is empty")
	}
}
