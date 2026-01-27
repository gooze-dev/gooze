package main

import "testing"

func TestAssignments(t *testing.T) {
	result := assignments()
	if result != 30 {
		t.Errorf("expected 30, got %d", result)
	}
}

func TestExpressions(t *testing.T) {
	expressions()
}

func TestDeferStatements(t *testing.T) {
	deferStatements()
}

func TestGoroutines(t *testing.T) {
	goroutines()
}

func TestChannels(t *testing.T) {
	ch := make(chan int, 1)
	channels(ch)
	if val := <-ch; val != 42 {
		t.Errorf("expected 42, got %d", val)
	}
}
