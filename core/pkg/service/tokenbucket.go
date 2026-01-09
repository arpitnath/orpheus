package service

import (
	"math"
	"sync"
	"time"
)

type TokenBucket struct {
	tokens     float64
	maxTokens  int
	refillRate float64
	lastUpdate time.Time
	mu         sync.Mutex
}

func NewTokenBucket(maxTokens int, refillRatePerSecond float64) *TokenBucket {
	return &TokenBucket{
		tokens:     float64(maxTokens),
		maxTokens:  maxTokens,
		refillRate: refillRatePerSecond,
		lastUpdate: time.Now(),
	}
}

func (tb *TokenBucket) TryConsume() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastUpdate).Seconds()

	tokensToAdd := elapsed * tb.refillRate
	tb.tokens = math.Min(tb.tokens+tokensToAdd, float64(tb.maxTokens))
	tb.lastUpdate = now

	if tb.tokens < 1.0 {
		return false
	}

	tb.tokens -= 1.0
	return true
}

func (tb *TokenBucket) Reset() {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.tokens = float64(tb.maxTokens)
	tb.lastUpdate = time.Now()
}
