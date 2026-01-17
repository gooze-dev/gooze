package controller

import "testing"

func TestStartOptions(t *testing.T) {
	cfg := &StartConfig{}
	WithEstimateMode()(cfg)
	if cfg.mode != ModeEstimate {
		t.Fatalf("WithEstimateMode() mode = %v, want %v", cfg.mode, ModeEstimate)
	}

	WithTestMode()(cfg)
	if cfg.mode != ModeTest {
		t.Fatalf("WithTestMode() mode = %v, want %v", cfg.mode, ModeTest)
	}
}
