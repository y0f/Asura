package monitor

import (
	"testing"
	"time"
)

func TestComputeAdaptiveInterval(t *testing.T) {
	base := 60 * time.Second

	tests := []struct {
		name          string
		consecSuccess int
		consecFail    int
		prevMult      float64
		wantMult      float64
		checkInterval func(t *testing.T, interval time.Duration)
	}{
		{
			name: "stable monitor slows down", consecSuccess: stableThreshold,
			prevMult: 1.0, wantMult: slowdownStep,
		},
		{
			name: "stable monitor compounds slowdown", consecSuccess: stableThreshold,
			prevMult: slowdownStep, wantMult: slowdownStep * slowdownStep,
		},
		{
			name: "stable cap at 2x base", consecSuccess: stableThreshold,
			prevMult: 1.9, wantMult: maxSlowdown,
		},
		{
			name: "interval never exceeds 2x base", consecSuccess: stableThreshold,
			prevMult: maxSlowdown, wantMult: maxSlowdown,
			checkInterval: func(t *testing.T, interval time.Duration) {
				if interval > time.Duration(float64(base)*maxSlowdown) {
					t.Fatalf("interval %v exceeds max", interval)
				}
			},
		},
		{
			name: "flapping snaps to fast", consecFail: 1,
			prevMult: 1.5, wantMult: speedupStep,
		},
		{
			name: "flapping at max slowdown snaps to fast", consecFail: 1,
			prevMult: maxSlowdown, wantMult: speedupStep,
		},
		{
			name: "just recovered returns normal", consecSuccess: 5,
			prevMult: 0.5, wantMult: 1.0,
		},
		{
			name: "actively failing stays normal", consecFail: 5,
			prevMult: 1.0, wantMult: 1.0,
		},
		{
			name: "actively failing does not slow down", consecFail: 10,
			prevMult: 1.0, wantMult: 1.0,
		},
		{
			name: "floor enforced at 5s", consecFail: 1,
			prevMult: 1.5,
			checkInterval: func(t *testing.T, interval time.Duration) {
				if interval < minInterval {
					t.Fatalf("interval %v below minimum %v", interval, minInterval)
				}
			},
		},
		{
			name: "zero prev multiplier treated as 1.0", consecSuccess: 10,
			prevMult: 0, wantMult: 1.0,
		},
		{
			name: "negative prev multiplier treated as 1.0", consecSuccess: 10,
			prevMult: -1.0, wantMult: 1.0,
		},
		{
			name: "exactly at stable threshold", consecSuccess: stableThreshold,
			prevMult: 1.0, wantMult: slowdownStep,
		},
		{
			name: "one below stable threshold stays normal", consecSuccess: stableThreshold - 1,
			prevMult: 1.0, wantMult: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testBase := base
			if tt.name == "floor enforced at 5s" {
				testBase = 6 * time.Second
			}
			interval, mult := computeAdaptiveInterval(testBase, tt.consecSuccess, tt.consecFail, tt.prevMult)
			if tt.wantMult > 0 && mult != tt.wantMult {
				t.Fatalf("expected multiplier %v, got %v", tt.wantMult, mult)
			}
			if tt.checkInterval != nil {
				tt.checkInterval(t, interval)
			}
		})
	}
}
