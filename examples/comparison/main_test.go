package main

import "testing"

func TestMax(t *testing.T) {
	tests := []struct {
		a, b, expected int
	}{
		{5, 10, 10},
		{10, 5, 10},
		{5, 5, 5},
		{-5, -10, -5},
	}

	for _, tt := range tests {
		result := Max(tt.a, tt.b)
		if result != tt.expected {
			t.Errorf("Max(%d, %d) = %d, expected %d", tt.a, tt.b, result, tt.expected)
		}
	}
}

func TestMin(t *testing.T) {
	tests := []struct {
		a, b, expected int
	}{
		{5, 10, 5},
		{10, 5, 5},
		{5, 5, 5},
		{-5, -10, -10},
	}

	for _, tt := range tests {
		result := Min(tt.a, tt.b)
		if result != tt.expected {
			t.Errorf("Min(%d, %d) = %d, expected %d", tt.a, tt.b, result, tt.expected)
		}
	}
}

func TestIsInRange(t *testing.T) {
	tests := []struct {
		value, min, max int
		expected        bool
	}{
		{7, 5, 10, true},
		{5, 5, 10, true},
		{10, 5, 10, true},
		{3, 5, 10, false},
		{12, 5, 10, false},
	}

	for _, tt := range tests {
		result := IsInRange(tt.value, tt.min, tt.max)
		if result != tt.expected {
			t.Errorf("IsInRange(%d, %d, %d) = %v, expected %v",
				tt.value, tt.min, tt.max, result, tt.expected)
		}
	}
}

func TestAreEqual(t *testing.T) {
	tests := []struct {
		a, b     int
		expected bool
	}{
		{5, 5, true},
		{5, 10, false},
		{-5, -5, true},
		{0, 0, true},
	}

	for _, tt := range tests {
		result := AreEqual(tt.a, tt.b)
		if result != tt.expected {
			t.Errorf("AreEqual(%d, %d) = %v, expected %v", tt.a, tt.b, result, tt.expected)
		}
	}
}

func TestAreDifferent(t *testing.T) {
	tests := []struct {
		a, b     int
		expected bool
	}{
		{5, 10, true},
		{5, 5, false},
		{-5, -10, true},
		{0, 0, false},
	}

	for _, tt := range tests {
		result := AreDifferent(tt.a, tt.b)
		if result != tt.expected {
			t.Errorf("AreDifferent(%d, %d) = %v, expected %v", tt.a, tt.b, result, tt.expected)
		}
	}
}

func TestIsPositive(t *testing.T) {
	tests := []struct {
		n        int
		expected bool
	}{
		{5, true},
		{0, false},
		{-5, false},
		{1, true},
	}

	for _, tt := range tests {
		result := IsPositive(tt.n)
		if result != tt.expected {
			t.Errorf("IsPositive(%d) = %v, expected %v", tt.n, result, tt.expected)
		}
	}
}

func TestIsNegative(t *testing.T) {
	tests := []struct {
		n        int
		expected bool
	}{
		{-5, true},
		{0, false},
		{5, false},
		{-1, true},
	}

	for _, tt := range tests {
		result := IsNegative(tt.n)
		if result != tt.expected {
			t.Errorf("IsNegative(%d) = %v, expected %v", tt.n, result, tt.expected)
		}
	}
}

func TestCompare(t *testing.T) {
	tests := []struct {
		a, b, expected int
	}{
		{5, 10, -1},
		{10, 5, 1},
		{5, 5, 0},
		{-5, -10, 1},
		{-10, -5, -1},
	}

	for _, tt := range tests {
		result := Compare(tt.a, tt.b)
		if result != tt.expected {
			t.Errorf("Compare(%d, %d) = %d, expected %d", tt.a, tt.b, result, tt.expected)
		}
	}
}
