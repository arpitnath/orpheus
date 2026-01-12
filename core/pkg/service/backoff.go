package service

import (
	"math"
	"math/rand"
	"time"
)

type BackoffCalculator struct {
	current    time.Duration
	initial    time.Duration
	max        time.Duration
	multiplier float64
	jitter     float64
}

func NewBackoffCalculator(initial, max time.Duration, multiplier, jitter float64) *BackoffCalculator {
	return &BackoffCalculator{
		current:    initial,
		initial:    initial,
		max:        max,
		multiplier: multiplier,
		jitter:     jitter,
	}
}

func (bc *BackoffCalculator) Next(exitCode int) time.Duration {
	if !shouldRestartForExitCode(exitCode) {
		return 0
	}

	base := bc.current

	if exitCode == 137 {
		base = time.Duration(math.Max(float64(base*2), float64(60*time.Second)))
	}

	next := time.Duration(float64(base) * bc.multiplier)
	if next > bc.max {
		next = bc.max
	}

	jitterAmount := float64(next) * bc.jitter * (rand.Float64()*2 - 1)
	final := next + time.Duration(jitterAmount)

	bc.current = next

	return final
}

func (bc *BackoffCalculator) Reset() {
	bc.current = bc.initial
}

func shouldRestartForExitCode(code int) bool {
	switch code {
	case 0, 143:
		return false
	default:
		return true
	}
}
