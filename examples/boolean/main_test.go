package main

import "testing"

func TestCheckStatus(t *testing.T) {
	// Test with active=true, disabled=false - should return true
	result := checkStatus(true, false)
	if !result {
		t.Errorf("expected true, got false")
	}

	// Test with active=false - should return false
	result = checkStatus(false, false)
	if result {
		t.Errorf("expected false, got true")
	}
}

func TestMain(t *testing.T) {
	// Just ensure main doesn't panic
	main()
}
