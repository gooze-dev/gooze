package main

import "testing"

func TestCalculate(t *testing.T) {
	result := Calculate(10, 5)
	if result == 0 {
		t.Error("Result should not be 0")
	}
}
