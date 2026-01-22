package service

import (
	"sync"
	"testing"
	"time"
)

func TestNewTokenBucket(t *testing.T) {
	tb := NewTokenBucket(5, 1.0) // 5 tokens, refill 1/second

	if tb == nil {
		t.Fatal("NewTokenBucket returned nil")
	}

	// Should start with max tokens
	if tb.tokens != 5.0 {
		t.Errorf("Expected tokens=5.0, got %f", tb.tokens)
	}

	if tb.maxTokens != 5 {
		t.Errorf("Expected maxTokens=5, got %d", tb.maxTokens)
	}

	if tb.refillRate != 1.0 {
		t.Errorf("Expected refillRate=1.0, got %f", tb.refillRate)
	}
}

func TestTryConsume_WithCapacity(t *testing.T) {
	tb := NewTokenBucket(3, 1.0)

	// Should succeed (has tokens)
	if !tb.TryConsume() {
		t.Error("TryConsume failed when tokens available")
	}

	// Verify token count decreased
	if tb.tokens != 2.0 {
		t.Errorf("Expected 2 tokens remaining, got %f", tb.tokens)
	}
}

func TestTryConsume_Exhausted(t *testing.T) {
	tb := NewTokenBucket(2, 1.0)

	// Consume all tokens
	tb.TryConsume() // 1 left
	tb.TryConsume() // 0 left

	// Should fail (no tokens)
	if tb.TryConsume() {
		t.Error("TryConsume succeeded when bucket empty")
	}

	// Verify at or below 0 (may be slightly negative due to refill timing)
	tb.mu.Lock()
	tokens := tb.tokens
	tb.mu.Unlock()

	if tokens > 0.01 {  // Allow small floating point error
		t.Errorf("Expected ~0 tokens, got %f", tokens)
	}
}

func TestTryConsume_Refill(t *testing.T) {
	tb := NewTokenBucket(5, 2.0) // Refill 2 tokens/second

	// Consume all tokens
	for i := 0; i < 5; i++ {
		tb.TryConsume()
	}

	// Verify exhausted
	if tb.TryConsume() {
		t.Error("Should be exhausted")
	}

	// Wait for refill (1 second = 2 tokens at 2/s rate)
	time.Sleep(1 * time.Second)

	// Should have ~2 tokens now
	if !tb.TryConsume() {
		t.Error("Should have refilled tokens after 1s")
	}

	// Consume should succeed again (1 token left)
	if !tb.TryConsume() {
		t.Error("Should have 2 tokens after refill")
	}

	// Third should fail (0 tokens now)
	if tb.TryConsume() {
		t.Error("Should be exhausted after consuming 2 refilled tokens")
	}
}

func TestTryConsume_Concurrent(t *testing.T) {
	tb := NewTokenBucket(100, 0.0) // No refill (test concurrency only)

	var wg sync.WaitGroup
	successCount := 0
	var mu sync.Mutex

	// 200 goroutines try to consume (only 100 should succeed)
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if tb.TryConsume() {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	// Exactly 100 should have succeeded (initial tokens)
	if successCount != 100 {
		t.Errorf("Expected exactly 100 successes, got %d", successCount)
	}

	// Bucket should be exhausted
	if tb.TryConsume() {
		t.Error("Bucket should be exhausted after 100 consumes")
	}
}

func TestTokenBucket_Reset(t *testing.T) {
	tb := NewTokenBucket(5, 1.0)

	// Consume all tokens
	for i := 0; i < 5; i++ {
		tb.TryConsume()
	}

	// Verify exhausted
	if tb.TryConsume() {
		t.Error("Should be exhausted before reset")
	}

	// Reset
	tb.Reset()

	// Should have max tokens again
	if tb.tokens != float64(tb.maxTokens) {
		t.Errorf("After reset, expected %d tokens, got %f", tb.maxTokens, tb.tokens)
	}

	// Consume should succeed
	if !tb.TryConsume() {
		t.Error("Should have tokens after reset")
	}
}

func TestTokenBucket_RefillCapping(t *testing.T) {
	tb := NewTokenBucket(5, 100.0) // Very high refill rate

	// Consume 1 token
	tb.TryConsume()

	// Wait long enough for many refills
	time.Sleep(2 * time.Second)

	// Should be capped at maxTokens (not 200+)
	tb.mu.Lock()
	tokens := tb.tokens
	tb.mu.Unlock()

	if tokens > float64(tb.maxTokens) {
		t.Errorf("Tokens should be capped at max=%d, got %f", tb.maxTokens, tokens)
	}
}
