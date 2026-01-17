package main

import "fmt"

// Max returns the maximum of two integers
func Max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Min returns the minimum of two integers
func Min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// IsInRange checks if a value is within a range
func IsInRange(value, min, max int) bool {
	return value >= min && value <= max
}

// AreEqual checks if two values are equal
func AreEqual(a, b int) bool {
	return a == b
}

// AreDifferent checks if two values are different
func AreDifferent(a, b int) bool {
	return a != b
}

// IsPositive checks if a number is positive
func IsPositive(n int) bool {
	return n > 0
}

// IsNegative checks if a number is negative
func IsNegative(n int) bool {
	return n < 0
}

// Compare returns 1 if a > b, -1 if a < b, 0 if equal
func Compare(a, b int) int {
	if a > b {
		return 1
	}
	if a < b {
		return -1
	}
	return 0
}

func main() {
	fmt.Println("Max(5, 10):", Max(5, 10))
	fmt.Println("Min(5, 10):", Min(5, 10))
	fmt.Println("IsInRange(7, 5, 10):", IsInRange(7, 5, 10))
	fmt.Println("AreEqual(5, 5):", AreEqual(5, 5))
	fmt.Println("AreDifferent(5, 10):", AreDifferent(5, 10))
	fmt.Println("IsPositive(5):", IsPositive(5))
	fmt.Println("IsNegative(-5):", IsNegative(-5))
	fmt.Println("Compare(5, 10):", Compare(5, 10))
}
