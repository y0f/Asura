package monitor

import (
	"testing"
	"time"
)

func TestComputeAdaptiveInterval(t *testing.T) {
	base := 60 * time.Second

	t.Run("stable monitor slows down", func(t *testing.T) {
		interval, multiplier := computeAdaptiveInterval(base, stableThreshold, 0, 1.0)
		if multiplier != slowdownStep {
			t.Fatalf("expected multiplier %v, got %v", slowdownStep, multiplier)
		}
		want := time.Duration(float64(base) * slowdownStep)
		if interval != want {
			t.Fatalf("expected interval %v, got %v", want, interval)
		}
	})

	t.Run("stable monitor compounds slowdown", func(t *testing.T) {
		// First slowdown
		_, m1 := computeAdaptiveInterval(base, stableThreshold, 0, 1.0)
		// Second slowdown
		interval, m2 := computeAdaptiveInterval(base, stableThreshold, 0, m1)
		wantMult := slowdownStep * slowdownStep
		if m2 != wantMult {
			t.Fatalf("expected multiplier %v, got %v", wantMult, m2)
		}
		want := time.Duration(float64(base) * wantMult)
		if interval != want {
			t.Fatalf("expected interval %v, got %v", want, interval)
		}
	})

	t.Run("stable cap at 2x base", func(t *testing.T) {
		_, multiplier := computeAdaptiveInterval(base, stableThreshold, 0, 1.9)
		if multiplier != maxSlowdown {
			t.Fatalf("expected multiplier capped at %v, got %v", maxSlowdown, multiplier)
		}
	})

	t.Run("interval never exceeds 2x base", func(t *testing.T) {
		interval, _ := computeAdaptiveInterval(base, stableThreshold, 0, maxSlowdown)
		maxInterval := time.Duration(float64(base) * maxSlowdown)
		if interval > maxInterval {
			t.Fatalf("interval %v exceeds max %v", interval, maxInterval)
		}
	})

	t.Run("flapping snaps to fast", func(t *testing.T) {
		// Was slowed down (multiplier > 1.0), then failure occurs
		interval, multiplier := computeAdaptiveInterval(base, 0, 1, 1.5)
		if multiplier != speedupStep {
			t.Fatalf("expected multiplier %v, got %v", speedupStep, multiplier)
		}
		want := time.Duration(float64(base) * speedupStep)
		if interval != want {
			t.Fatalf("expected interval %v, got %v", want, interval)
		}
	})

	t.Run("flapping at max slowdown snaps to fast", func(t *testing.T) {
		interval, multiplier := computeAdaptiveInterval(base, 0, 1, maxSlowdown)
		if multiplier != speedupStep {
			t.Fatalf("expected multiplier %v, got %v", speedupStep, multiplier)
		}
		want := time.Duration(float64(base) * speedupStep)
		if interval != want {
			t.Fatalf("expected interval %v, got %v", want, interval)
		}
	})

	t.Run("just recovered returns normal", func(t *testing.T) {
		interval, multiplier := computeAdaptiveInterval(base, 5, 0, 0.5)
		if multiplier != 1.0 {
			t.Fatalf("expected multiplier 1.0, got %v", multiplier)
		}
		if interval != base {
			t.Fatalf("expected interval %v, got %v", base, interval)
		}
	})

	t.Run("actively failing stays normal", func(t *testing.T) {
		interval, multiplier := computeAdaptiveInterval(base, 0, 5, 1.0)
		if multiplier != 1.0 {
			t.Fatalf("expected multiplier 1.0, got %v", multiplier)
		}
		if interval != base {
			t.Fatalf("expected interval %v, got %v", base, interval)
		}
	})

	t.Run("actively failing does not slow down", func(t *testing.T) {
		// Already at normal, keeps failing
		_, multiplier := computeAdaptiveInterval(base, 0, 10, 1.0)
		if multiplier != 1.0 {
			t.Fatalf("expected multiplier 1.0 during active failure, got %v", multiplier)
		}
	})

	t.Run("floor enforced at 5s", func(t *testing.T) {
		// Very short base interval with speedup
		shortBase := 6 * time.Second
		interval, _ := computeAdaptiveInterval(shortBase, 0, 1, 1.5)
		if interval < minInterval {
			t.Fatalf("interval %v below minimum %v", interval, minInterval)
		}
	})

	t.Run("extremely short base clamped to 5s", func(t *testing.T) {
		tinyBase := 5 * time.Second
		interval, _ := computeAdaptiveInterval(tinyBase, 0, 1, 2.0)
		if interval < minInterval {
			t.Fatalf("interval %v below minimum %v", interval, minInterval)
		}
	})

	t.Run("zero prev multiplier treated as 1.0", func(t *testing.T) {
		interval, multiplier := computeAdaptiveInterval(base, 10, 0, 0)
		if multiplier != 1.0 {
			t.Fatalf("expected multiplier 1.0 for zero prev, got %v", multiplier)
		}
		if interval != base {
			t.Fatalf("expected interval %v, got %v", base, interval)
		}
	})

	t.Run("negative prev multiplier treated as 1.0", func(t *testing.T) {
		_, multiplier := computeAdaptiveInterval(base, 10, 0, -1.0)
		if multiplier != 1.0 {
			t.Fatalf("expected multiplier 1.0 for negative prev, got %v", multiplier)
		}
	})

	t.Run("exactly at stable threshold", func(t *testing.T) {
		_, multiplier := computeAdaptiveInterval(base, stableThreshold, 0, 1.0)
		if multiplier != slowdownStep {
			t.Fatalf("expected slowdown at exact threshold, got %v", multiplier)
		}
	})

	t.Run("one below stable threshold stays normal", func(t *testing.T) {
		_, multiplier := computeAdaptiveInterval(base, stableThreshold-1, 0, 1.0)
		if multiplier != 1.0 {
			t.Fatalf("expected normal below threshold, got %v", multiplier)
		}
	})
}
