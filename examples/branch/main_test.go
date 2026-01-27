package main

import "testing"

func TestMax(t *testing.T) {
	tests := []struct {
		a, b, expected int
	}{
		{5, 3, 5},
		{2, 7, 7},
		{4, 4, 4},
	}

	for _, tt := range tests {
		result := max(tt.a, tt.b)
		if result != tt.expected {
			t.Errorf("max(%d, %d) = %d, want %d", tt.a, tt.b, result, tt.expected)
		}
	}
}

func TestCheckStatus(t *testing.T) {
	tests := []struct {
		status, expected string
	}{
		{"active", "running"},
		{"inactive", "stopped"},
		{"pending", "unknown"},
	}

	for _, tt := range tests {
		result := checkStatus(tt.status)
		if result != tt.expected {
			t.Errorf("checkStatus(%q) = %q, want %q", tt.status, result, tt.expected)
		}
	}
}

func TestProcessValue(t *testing.T) {
	tests := []struct {
		input, expected int
	}{
		{150, 100},
		{50, 50},
		{-10, 0},
	}

	for _, tt := range tests {
		result := processValue(tt.input)
		if result != tt.expected {
			t.Errorf("processValue(%d) = %d, want %d", tt.input, result, tt.expected)
		}
	}
}

func TestLoopExample(t *testing.T) {
	result := loopExample()
	expected := 45 // sum of 0 to 9
	if result != expected {
		t.Errorf("loopExample() = %d, want %d", result, expected)
	}
}
