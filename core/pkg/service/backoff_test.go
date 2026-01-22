package service

import (
	"math"
	"testing"
	"time"
)

func TestNewBackoffCalculator(t *testing.T) {
	bc := NewBackoffCalculator(2*time.Second, 60*time.Second, 2.0, 0.2)

	if bc == nil {
		t.Fatal("NewBackoffCalculator returned nil")
	}

	if bc.initial != 2*time.Second {
		t.Errorf("Expected initial=2s, got %v", bc.initial)
	}

	if bc.max != 60*time.Second {
		t.Errorf("Expected max=60s, got %v", bc.max)
	}

	if bc.multiplier != 2.0 {
		t.Errorf("Expected multiplier=2.0, got %f", bc.multiplier)
	}

	if bc.jitter != 0.2 {
		t.Errorf("Expected jitter=0.2, got %f", bc.jitter)
	}

	// Should start at initial
	if bc.current != 2*time.Second {
		t.Errorf("Expected current=2s, got %v", bc.current)
	}
}

func TestNext_CleanExit(t *testing.T) {
	bc := NewBackoffCalculator(2*time.Second, 60*time.Second, 2.0, 0.0) // No jitter for predictability

	delay := bc.Next(0) // Exit code 0 = clean exit

	if delay != 0 {
		t.Errorf("Clean exit (0) should return 0 delay, got %v", delay)
	}

	// Current should not advance
	if bc.current != 2*time.Second {
		t.Errorf("Current should stay at initial after clean exit, got %v", bc.current)
	}
}

func TestNext_SIGTERM(t *testing.T) {
	bc := NewBackoffCalculator(2*time.Second, 60*time.Second, 2.0, 0.0)

	delay := bc.Next(143) // Exit code 143 = SIGTERM

	if delay != 0 {
		t.Errorf("SIGTERM (143) should return 0 delay, got %v", delay)
	}
}

func TestNext_OOM(t *testing.T) {
	bc := NewBackoffCalculator(2*time.Second, 60*time.Second, 2.0, 0.0)

	delay := bc.Next(137) // Exit code 137 = OOM SIGKILL

	// OOM should get minimum 60s backoff
	if delay < 60*time.Second {
		t.Errorf("OOM should get min 60s backoff, got %v", delay)
	}

	// Should be max(current*2, 60s)
	expected := 60 * time.Second // max(2s*2, 60s) = 60s
	if delay != expected {
		t.Errorf("OOM backoff expected %v, got %v", expected, delay)
	}
}

func TestNext_NormalCrash(t *testing.T) {
	bc := NewBackoffCalculator(2*time.Second, 60*time.Second, 2.0, 0.0) // No jitter

	delay := bc.Next(1) // Normal crash

	// Returns next value: 2s * 2.0 = 4s
	if delay != 4*time.Second {
		t.Errorf("First crash should return 4s (2s * 2.0), got %v", delay)
	}

	// Current should advance to 4s
	if bc.current != 4*time.Second {
		t.Errorf("After first crash, current should be 4s, got %v", bc.current)
	}
}

func TestNext_Progression(t *testing.T) {
	bc := NewBackoffCalculator(2*time.Second, 60*time.Second, 2.0, 0.0) // No jitter

	// Backoff returns NEXT value (advances before returning)
	// current=2s → next=2s*2=4s → return 4s → current becomes 4s
	// current=4s → next=4s*2=8s → return 8s → current becomes 8s
	expected := []time.Duration{
		4 * time.Second,  // 2s * 2 = 4s
		8 * time.Second,  // 4s * 2 = 8s
		16 * time.Second, // 8s * 2 = 16s
		32 * time.Second, // 16s * 2 = 32s
		60 * time.Second, // 32s * 2 = 64s (clamped to max 60s)
		60 * time.Second, // 60s * 2 = 120s (clamped to max 60s)
	}

	for i, exp := range expected {
		delay := bc.Next(1) // Normal crash
		if delay != exp {
			t.Errorf("Crash %d: expected %v, got %v", i+1, exp, delay)
		}
	}
}

func TestNext_Jitter(t *testing.T) {
	bc := NewBackoffCalculator(10*time.Second, 60*time.Second, 2.0, 0.2) // 20% jitter

	delays := make([]time.Duration, 10)
	for i := 0; i < 10; i++ {
		bc.Reset()
		delays[i] = bc.Next(1)
	}

	// Returns next value: 10s * 2.0 = 20s
	// Jitter ±20% of 20s → range [16s, 24s]
	base := 20 * time.Second  // 10s * 2.0 multiplier
	minDelay := time.Duration(float64(base) * 0.8)  // 16s
	maxDelay := time.Duration(float64(base) * 1.2) // 24s

	for i, delay := range delays {
		if delay < minDelay || delay > maxDelay {
			t.Errorf("Delay %d (%v) outside jitter range [%v, %v]", i, delay, minDelay, maxDelay)
		}
	}

	// Verify delays vary (not all the same)
	allSame := true
	for i := 1; i < len(delays); i++ {
		if delays[i] != delays[0] {
			allSame = false
			break
		}
	}

	if allSame {
		t.Error("Jitter should cause variation, but all delays were identical")
	}
}

func TestNext_MaxClamping(t *testing.T) {
	bc := NewBackoffCalculator(30*time.Second, 60*time.Second, 2.0, 0.0)

	// First: 30s
	bc.Next(1)
	// Second: 60s (clamped)
	delay := bc.Next(1)

	if delay > 60*time.Second {
		t.Errorf("Delay should be clamped to max=60s, got %v", delay)
	}

	// Even accounting for jitter (0 in this test), should not exceed max
	if delay != 60*time.Second {
		t.Errorf("Expected exactly 60s (no jitter), got %v", delay)
	}
}

func TestBackoffCalculator_Reset(t *testing.T) {
	bc := NewBackoffCalculator(2*time.Second, 60*time.Second, 2.0, 0.0)

	// Advance backoff
	bc.Next(1) // 2s
	bc.Next(1) // 4s
	bc.Next(1) // 8s

	// current should be 16s now
	if bc.current != 16*time.Second {
		t.Errorf("Before reset, expected current=16s, got %v", bc.current)
	}

	// Reset
	bc.Reset()

	// Should be back to initial
	if bc.current != bc.initial {
		t.Errorf("After reset, expected current=%v, got %v", bc.initial, bc.current)
	}

	// Next call should return next value (initial * multiplier)
	delay := bc.Next(1)
	if delay != 4*time.Second {  // 2s * 2.0 = 4s
		t.Errorf("After reset, expected 4s (2s * 2.0), got %v", delay)
	}
}

func TestNext_OOM_DoubleBackoff(t *testing.T) {
	bc := NewBackoffCalculator(4*time.Second, 120*time.Second, 2.0, 0.0)

	// Normal crash: 4s → current becomes 8s
	bc.Next(1)

	// OOM: base=max(8s*2, 60s)=60s, next=60s*2.0=120s
	delay := bc.Next(137)

	// Should return 120s (base 60s * multiplier 2.0)
	if delay != 120*time.Second {
		t.Errorf("OOM after 4s backoff should be 120s, got %v", delay)
	}

	// Current should now be 120s
	if bc.current != 120*time.Second {
		t.Errorf("After OOM, current should be 120s, got %v", bc.current)
	}
}

func TestBackoffCalculator_JitterBounds(t *testing.T) {
	bc := NewBackoffCalculator(100*time.Millisecond, 1*time.Second, 2.0, 0.5) // 50% jitter

	// Run 100 times, verify all within bounds
	for i := 0; i < 100; i++ {
		bc.Reset()
		delay := bc.Next(1)

		// Returns next value: 100ms * 2.0 = 200ms
		// Jitter ±50% of 200ms → range [100ms, 300ms]
		baseDelay := 200 * time.Millisecond // 100ms * 2.0
		minExpected := time.Duration(float64(baseDelay) * 0.5)  // 100ms
		maxExpected := time.Duration(float64(baseDelay) * 1.5)  // 300ms

		if delay < minExpected || delay > maxExpected {
			t.Errorf("Delay %v outside jitter bounds [%v, %v]", delay, minExpected, maxExpected)
		}
	}
}

func TestBackoffCalculator_ZeroJitter(t *testing.T) {
	bc := NewBackoffCalculator(5*time.Second, 60*time.Second, 2.0, 0.0) // No jitter

	delays := make([]time.Duration, 5)
	for i := 0; i < 5; i++ {
		bc.Reset()
		delays[i] = bc.Next(1)
	}

	// All should be exactly the same (deterministic)
	for i := 1; i < len(delays); i++ {
		if delays[i] != delays[0] {
			t.Errorf("With zero jitter, all delays should be identical: %v != %v", delays[i], delays[0])
		}
	}

	// Returns next value: 5s * 2.0 = 10s
	if delays[0] != 10*time.Second {
		t.Errorf("Expected exactly 10s (5s * 2.0 multiplier), got %v", delays[0])
	}
}

func TestBackoffCalculator_FloatPrecision(t *testing.T) {
	bc := NewBackoffCalculator(1*time.Second, 60*time.Second, 1.5, 0.0)

	// Backoff returns NEXT value (current * multiplier)
	// current=1s → next=1s*1.5=1.5s → current becomes 1.5s
	// current=1.5s → next=1.5s*1.5=2.25s → current becomes 2.25s
	expected := []time.Duration{
		time.Duration(1.5 * float64(time.Second)),      // 1s * 1.5
		time.Duration(2.25 * float64(time.Second)),     // 1.5s * 1.5
		time.Duration(3.375 * float64(time.Second)),    // 2.25s * 1.5
		time.Duration(5.0625 * float64(time.Second)),   // 3.375s * 1.5
		time.Duration(7.59375 * float64(time.Second)),  // 5.0625s * 1.5
	}

	for i, exp := range expected {
		delay := bc.Next(1)

		// Allow small floating point error (1ms)
		diff := time.Duration(math.Abs(float64(delay - exp)))
		if diff > time.Millisecond {
			t.Errorf("Iteration %d: expected %v, got %v (diff=%v)", i, exp, delay, diff)
		}
	}
}
