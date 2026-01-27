package main

func standardForLoop(n int) int {
	sum := 0
	for i := 0; i < n; i++ {
		sum += i
	}
	return sum
}

func rangeLoopSlice(items []int) int {
	sum := 0
	for _, item := range items {
		sum += item
	}
	return sum
}

func loopWithBreak(n int) int {
	sum := 0
	for i := 0; i < n; i++ {
		if i == 5 {
			break
		}
		sum += i
	}
	return sum
}

func loopWithContinue(n int) int {
	sum := 0
	for i := 0; i < n; i++ {
		if i%2 == 0 {
			continue
		}
		sum += i
	}
	return sum
}

func whileStyleLoop(x int) int {
	count := 0
	for x > 0 {
		x--
		count++
	}
	return count
}

func nestedLoops(n int) int {
	sum := 0
	for i := 0; i < n; i++ {
		for j := 0; j <= i; j++ {
			sum++
		}
	}
	return sum
}

func rangeLoopMap(m map[string]int) int {
	sum := 0
	for _, v := range m {
		sum += v
	}
	return sum
}

// Recursive function - can be replaced with loop
func factorialRecursive(n int) int {
	if n <= 1 {
		return 1
	}
	return n * factorialRecursive(n-1)
}

// Iterative version using loop
func factorialIterative(n int) int {
	result := 1
	for i := 2; i <= n; i++ {
		result *= i
	}
	return result
}

// Recursive sum
func sumRecursive(n int) int {
	if n <= 0 {
		return 0
	}
	return n + sumRecursive(n-1)
}

// Iterative sum
func sumIterative(n int) int {
	sum := 0
	for i := 1; i <= n; i++ {
		sum += i
	}
	return sum
}

// Fibonacci recursive
func fibonacciRecursive(n int) int {
	if n <= 1 {
		return n
	}
	return fibonacciRecursive(n-1) + fibonacciRecursive(n-2)
}

// Fibonacci iterative
func fibonacciIterative(n int) int {
	if n <= 1 {
		return n
	}
	prev, curr := 0, 1
	for i := 2; i <= n; i++ {
		prev, curr = curr, prev+curr
	}
	return curr
}
