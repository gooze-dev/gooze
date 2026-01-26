package main

func Calculate(a, b int) int {
	// Multiple arithmetic operations to generate multiple mutations
	x := a + b  // Mutation ID 0: + can become -, *, /, %
	y := a - b  // Mutation ID 5: - can become +, *, /, %
	z := a * b  // Mutation ID 10: * can become +, -, /, %
	w := a / b  // Mutation ID 15: / can become +, -, *, %

	// Comparison operator
	if x > y {  // Mutation ID 20: > can become <, <=, >=, ==, !=
		return z
	}
	return w
}

func main() {
	_ = Calculate(10, 5)
}
