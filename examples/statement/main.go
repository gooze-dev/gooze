package main

import "fmt"

func assignments() int {
	x := 10
	y := 20
	z := x + y
	return z
}

func expressions() {
	fmt.Println("hello")
	fmt.Printf("world")
	doWork()
}

func deferStatements() {
	defer cleanup()
	doWork()
}

func goroutines() {
	go worker()
	process()
}

func channels(ch chan int) {
	ch <- 42
	doWork()
}

func cleanup() {}
func doWork()  {}
func worker()  {}
func process() {}
