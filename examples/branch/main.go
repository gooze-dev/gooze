package main

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func checkStatus(status string) string {
	switch status {
	case "active":
		return "running"
	case "inactive":
		return "stopped"
	default:
		return "unknown"
	}
}

func processValue(x int) int {
	if x > 100 {
		return 100
	} else if x < 0 {
		return 0
	} else {
		return x
	}
}

func loopExample() int {
	sum := 0
	for i := 0; i < 10; i++ {
		sum += i
	}
	return sum
}
