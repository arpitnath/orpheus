package auth

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimiter enforces per-key rate limits using token bucket algorithm.
type RateLimiter struct {
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex

	// Cleanup interval
	cleanupInterval time.Duration
	lastAccess      map[string]time.Time
	done            chan struct{}
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter() *RateLimiter {
	rl := &RateLimiter{
		limiters:        make(map[string]*rate.Limiter),
		lastAccess:      make(map[string]time.Time),
		cleanupInterval: 5 * time.Minute,
		done:            make(chan struct{}),
	}

	// Start cleanup goroutine
	go rl.cleanupLoop()

	return rl
}

// Allow checks if a request is allowed for the given API key.
// Returns true if allowed, false if rate limit exceeded.
func (rl *RateLimiter) Allow(apiKey string, requestsPerMinute int) bool {
	limiter := rl.getLimiter(apiKey, requestsPerMinute)

	// Update last access time
	rl.mu.Lock()
	rl.lastAccess[apiKey] = time.Now()
	rl.mu.Unlock()

	return limiter.Allow()
}

// getLimiter gets or creates a rate limiter for an API key.
func (rl *RateLimiter) getLimiter(apiKey string, requestsPerMinute int) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	limiter, exists := rl.limiters[apiKey]
	if !exists {
		// Create new limiter
		// Rate: requests per second (rpm / 60)
		// Burst: Allow bursts up to rpm / 6 (10 seconds worth)
		r := rate.Limit(float64(requestsPerMinute) / 60.0)
		burst := max(requestsPerMinute/6, 10) // At least 10

		limiter = rate.NewLimiter(r, burst)
		rl.limiters[apiKey] = limiter
		rl.lastAccess[apiKey] = time.Now()
	}

	return limiter
}

// cleanupLoop periodically removes inactive rate limiters.
func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.cleanup()
		case <-rl.done:
			return
		}
	}
}

// cleanup removes limiters that haven't been used recently.
func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Remove limiters inactive for > 1 hour
	cutoff := time.Now().Add(-1 * time.Hour)

	for key, lastAccess := range rl.lastAccess {
		if lastAccess.Before(cutoff) {
			delete(rl.limiters, key)
			delete(rl.lastAccess, key)
		}
	}
}

// Stop stops the cleanup goroutine.
func (rl *RateLimiter) Stop() {
	close(rl.done)
}

// max returns the maximum of two integers.
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
