package controller

import "testing"

func TestFileItem_FilterValue(t *testing.T) {
	item := fileItem{path: "path/to/file.go", count: 2}
	if got := item.FilterValue(); got != item.path {
		t.Fatalf("FilterValue() = %q, want %q", got, item.path)
	}
}
