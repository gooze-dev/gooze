package main

import "fmt"

func IsPositive(x int) bool {
	return x > 0
}

func main() {
	fmt.Println(IsPositive(5))
	fmt.Println(IsPositive(-3))
}
