package main

import "fmt"

// Global constants - mutation testing for boolean literals, numbers
const (
	MaxRetries    = 3
	EnableLogging = true
	Pi            = 3.14159
)

// Global variables - can be mutated
var (
	counter   = 0
	isEnabled = false
)

// init function - all mutations apply here
func init() {
	counter = 10
	if isEnabled {
		fmt.Println("Initialized")
	}
}

// Regular functions - function-specific mutations
func Calculate(a, b int) int {
	if a > b {
		return a + b
	}
	return a - b
}

func Validate(value int) bool {
	return value > 0 && value <= MaxRetries
}

func main() {
	result := Calculate(5, 3)
	fmt.Printf("Result: %d\n", result)

	if Validate(result) {
		fmt.Println("Valid")
	}
}
