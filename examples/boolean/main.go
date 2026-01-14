package main

import "fmt"

func main() {
	// Boolean literals for testing
	isValid := true
	isComplete := false

	if isValid {
		fmt.Println("Valid")
	}

	if !isComplete {
		fmt.Println("Not complete")
	}

	result := checkStatus(true, false)
	fmt.Println(result)
}

func checkStatus(active bool, disabled bool) bool {
	return active && !disabled
}
