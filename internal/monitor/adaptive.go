package monitor

import "time"

const (
	stableThreshold = 60
	maxSlowdown     = 2.0
	maxSpeedup      = 0.5
	slowdownStep    = 1.25
	speedupStep     = 0.5
	minInterval     = 5 * time.Second
)

func computeAdaptiveInterval(baseInterval time.Duration, consecSuccesses int, consecFails int, prevMultiplier float64) (time.Duration, float64) {
	multiplier := prevMultiplier
	if multiplier <= 0 {
		multiplier = 1.0
	}

	switch {
	case consecSuccesses >= stableThreshold:
		// Stable: gradually slow down, capped at 2x base
		multiplier = multiplier * slowdownStep
		if multiplier > maxSlowdown {
			multiplier = maxSlowdown
		}

	case consecFails > 0 && consecSuccesses == 0 && prevMultiplier > 1.0:
		// Was slowed down and just failed: snap to fast checking
		multiplier = speedupStep

	case consecSuccesses > 0 && consecSuccesses < stableThreshold:
		// Recently recovered or not yet stable: normal base
		multiplier = 1.0

	case consecFails > 0:
		// Actively failing: use normal interval
		multiplier = 1.0
	}

	interval := time.Duration(float64(baseInterval) * multiplier)

	maxInterval := time.Duration(float64(baseInterval) * maxSlowdown)
	if interval > maxInterval {
		interval = maxInterval
	}
	if interval < minInterval {
		interval = minInterval
	}

	return interval, multiplier
}
