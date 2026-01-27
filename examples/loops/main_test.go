package main

import "testing"

func TestStandardForLoop(t *testing.T) {
	result := standardForLoop(5)
	expected := 10 // 0+1+2+3+4
	if result != expected {
		t.Errorf("standardForLoop(5) = %d; want %d", result, expected)
	}
}

func TestRangeLoopSlice(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}
	result := rangeLoopSlice(items)
	expected := 15
	if result != expected {
		t.Errorf("rangeLoopSlice(%v) = %d; want %d", items, result, expected)
	}
}

func TestLoopWithBreak(t *testing.T) {
	result := loopWithBreak(10)
	expected := 10 // 0+1+2+3+4 (stops at 5)
	if result != expected {
		t.Errorf("loopWithBreak(10) = %d; want %d", result, expected)
	}
}

func TestLoopWithContinue(t *testing.T) {
	result := loopWithContinue(6)
	expected := 9 // 1+3+5 (skips even numbers)
	if result != expected {
		t.Errorf("loopWithContinue(6) = %d; want %d", result, expected)
	}
}

func TestWhileStyleLoop(t *testing.T) {
	result := whileStyleLoop(5)
	expected := 5
	if result != expected {
		t.Errorf("whileStyleLoop(5) = %d; want %d", result, expected)
	}
}

func TestNestedLoops(t *testing.T) {
	result := nestedLoops(4)
	expected := 10 // Sum of (1 + 2 + 3 + 4)
	if result != expected {
		t.Errorf("nestedLoops(4) = %d; want %d", result, expected)
	}
}

func TestRangeLoopMap(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2, "c": 3}
	result := rangeLoopMap(m)
	expected := 6
	if result != expected {
		t.Errorf("rangeLoopMap() = %d; want %d", result, expected)
	}
}

func TestFactorialRecursive(t *testing.T) {
	tests := []struct {
		input, expected int
	}{
		{0, 1},
		{1, 1},
		{5, 120},
		{7, 5040},
	}

	for _, tt := range tests {
		result := factorialRecursive(tt.input)
		if result != tt.expected {
			t.Errorf("factorialRecursive(%d) = %d; want %d", tt.input, result, tt.expected)
		}
	}
}

func TestFactorialIterative(t *testing.T) {
	tests := []struct {
		input, expected int
	}{
		{0, 1},
		{1, 1},
		{5, 120},
		{7, 5040},
	}

	for _, tt := range tests {
		result := factorialIterative(tt.input)
		if result != tt.expected {
			t.Errorf("factorialIterative(%d) = %d; want %d", tt.input, result, tt.expected)
		}
	}
}

func TestSumRecursive(t *testing.T) {
	result := sumRecursive(10)
	expected := 55
	if result != expected {
		t.Errorf("sumRecursive(10) = %d; want %d", result, expected)
	}
}

func TestSumIterative(t *testing.T) {
	result := sumIterative(10)
	expected := 55
	if result != expected {
		t.Errorf("sumIterative(10) = %d; want %d", result, expected)
	}
}

func TestFibonacciRecursive(t *testing.T) {
	tests := []struct {
		input, expected int
	}{
		{0, 0},
		{1, 1},
		{5, 5},
		{10, 55},
	}

	for _, tt := range tests {
		result := fibonacciRecursive(tt.input)
		if result != tt.expected {
			t.Errorf("fibonacciRecursive(%d) = %d; want %d", tt.input, result, tt.expected)
		}
	}
}

func TestFibonacciIterative(t *testing.T) {
	tests := []struct {
		input, expected int
	}{
		{0, 0},
		{1, 1},
		{5, 5},
		{10, 55},
	}

	for _, tt := range tests {
		result := fibonacciIterative(tt.input)
		if result != tt.expected {
			t.Errorf("fibonacciIterative(%d) = %d; want %d", tt.input, result, tt.expected)
		}
	}
}
