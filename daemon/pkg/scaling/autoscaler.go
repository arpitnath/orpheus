package scaling

import (
	"context"
	"log"
	"math"
	"sync"
	"time"
)

// DefaultScalingInterval is the default time between scaling checks.
const DefaultScalingInterval = 5 * time.Second

// BasicAutoscaler implements the Autoscaler interface.
// It monitors queue metrics and adjusts worker pool sizes based on utilization.
//
// The core algorithm is queue-based scaling:
//
//	utilization = (pending + processing) / currentWorkers
//	if utilization > ScaleUpThreshold → scale up
//	if utilization < ScaleDownThreshold → scale down
//
// This is designed for extensibility - the calculateDesiredSize method
// contains the core logic that can be customized in future versions.
type BasicAutoscaler struct {
	// Pool management (keyed by agentID)
	pools    map[string]WorkerPool
	policies map[string]ScalingPolicy
	metrics  map[string]QueueMetrics
	mu       sync.RWMutex

	// Rate limiting state
	lastScaleTime map[string]time.Time
	lastScaleUp   map[string]bool // true if last scale was up, false if down

	// Configuration
	interval time.Duration

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// State
	running bool
}

// NewAutoscaler creates a new autoscaler with default settings.
func NewAutoscaler() *BasicAutoscaler {
	return NewAutoscalerWithInterval(DefaultScalingInterval)
}

// NewAutoscalerWithInterval creates a new autoscaler with a custom check interval.
func NewAutoscalerWithInterval(interval time.Duration) *BasicAutoscaler {
	return &BasicAutoscaler{
		pools:         make(map[string]WorkerPool),
		policies:      make(map[string]ScalingPolicy),
		metrics:       make(map[string]QueueMetrics),
		lastScaleTime: make(map[string]time.Time),
		lastScaleUp:   make(map[string]bool),
		interval:      interval,
	}
}

// Start begins the scaling loop that periodically evaluates pools.
func (a *BasicAutoscaler) Start(ctx context.Context) error {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return nil // Already running
	}

	a.ctx, a.cancel = context.WithCancel(ctx)
	a.running = true
	a.mu.Unlock()

	a.wg.Add(1)
	go a.scalingLoop()

	log.Printf("[autoscaler] Started with interval=%v", a.interval)
	return nil
}

// Stop halts the scaling loop.
func (a *BasicAutoscaler) Stop() error {
	a.mu.Lock()
	if !a.running {
		a.mu.Unlock()
		return nil // Not running
	}
	a.running = false
	a.mu.Unlock()

	if a.cancel != nil {
		a.cancel()
	}
	a.wg.Wait()

	log.Printf("[autoscaler] Stopped")
	return nil
}

// RegisterPool adds a worker pool to be managed by the autoscaler.
func (a *BasicAutoscaler) RegisterPool(agentID string, pool WorkerPool, policy ScalingPolicy) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.pools[agentID] = pool
	a.policies[agentID] = policy
	a.lastScaleTime[agentID] = time.Now()

	log.Printf("[autoscaler] Registered pool for %s (min=%d, max=%d, interval=%v)",
		agentID, policy.MinWorkers, policy.MaxWorkers, a.interval)
}

// UnregisterPool removes a pool from autoscaler management.
func (a *BasicAutoscaler) UnregisterPool(agentID string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.pools, agentID)
	delete(a.policies, agentID)
	delete(a.metrics, agentID)
	delete(a.lastScaleTime, agentID)
	delete(a.lastScaleUp, agentID)

	log.Printf("[autoscaler] Unregistered pool for %s", agentID)
}

// RegisterQueueMetrics provides queue depth information for scaling decisions.
func (a *BasicAutoscaler) RegisterQueueMetrics(agentID string, metrics QueueMetrics) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.metrics[agentID] = metrics

	log.Printf("[autoscaler] Registered queue metrics for %s", agentID)
}

// scalingLoop is the main goroutine that periodically checks for scaling.
func (a *BasicAutoscaler) scalingLoop() {
	defer a.wg.Done()

	ticker := time.NewTicker(a.interval)
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			a.performScalingCheck()
		}
	}
}

// performScalingCheck evaluates all registered pools and applies scaling decisions.
func (a *BasicAutoscaler) performScalingCheck() {
	a.mu.RLock()
	defer a.mu.RUnlock()

	for agentID, pool := range a.pools {
		metrics, hasMetrics := a.metrics[agentID]
		if !hasMetrics {
			continue // Skip if no metrics registered
		}

		policy := a.policies[agentID]

		// Rate limit check
		if !a.canScale(agentID, policy) {
			continue
		}

		// Get current state
		currentSize := pool.Size()
		pending := metrics.PendingTasks()
		processing := metrics.ProcessingTasks()

		// Calculate desired size using the core algorithm
		desiredSize := a.calculateDesiredSize(currentSize, pending, processing, policy)

		// Apply if different
		if desiredSize != currentSize {
			a.applyScaling(agentID, pool, currentSize, desiredSize, pending, processing)
		}
	}
}

// calculateDesiredSize is the core scaling algorithm.
// This method is isolated for extensibility - custom scaling strategies
// can override this logic in future versions.
//
// Algorithm:
//
//	utilization = (pending + processing) / currentWorkers
//	Scale UP if utilization > ScaleUpThreshold
//	Scale DOWN if utilization < ScaleDownThreshold
func (a *BasicAutoscaler) calculateDesiredSize(
	currentSize int,
	pending int,
	processing int,
	policy ScalingPolicy,
) int {
	// Edge case: no workers - bootstrap to minimum
	if currentSize == 0 {
		return policy.MinWorkers
	}

	// Calculate utilization: tasks per worker
	totalTasks := pending + processing
	utilization := float64(totalTasks) / float64(currentSize)

	// Scale UP decision
	if utilization > policy.ScaleUpThreshold {
		// Target: enough workers to achieve TargetUtilization
		target := int(math.Ceil(float64(totalTasks) / policy.TargetUtilization))

		// Ensure we scale up by at least 1
		if target <= currentSize {
			target = currentSize + 1
		}

		// Clamp to max
		if target > policy.MaxWorkers {
			target = policy.MaxWorkers
		}

		return target
	}

	// Scale DOWN decision
	if utilization < policy.ScaleDownThreshold {
		// Target: enough workers to achieve TargetUtilization
		target := int(math.Ceil(float64(totalTasks) / policy.TargetUtilization))

		// Conservative: don't scale down unless calculation clearly suggests it
		if target >= currentSize {
			return currentSize
		}

		// Clamp to min
		if target < policy.MinWorkers {
			target = policy.MinWorkers
		}

		return target
	}

	// No change - utilization is within acceptable range
	return currentSize
}

// canScale checks if enough time has passed since the last scaling operation.
// Uses direction-aware delays: ScaleUpDelay after scaling up, ScaleDownDelay after scaling down.
func (a *BasicAutoscaler) canScale(agentID string, policy ScalingPolicy) bool {
	lastScale, exists := a.lastScaleTime[agentID]
	if !exists {
		return true // First time, allow scaling
	}

	elapsed := time.Since(lastScale)

	// Check direction-aware delay
	wasScaleUp, hasDirection := a.lastScaleUp[agentID]
	if !hasDirection {
		// No direction recorded, use scale-up delay as default
		return elapsed >= policy.ScaleUpDelay
	}

	if wasScaleUp {
		return elapsed >= policy.ScaleUpDelay
	}
	return elapsed >= policy.ScaleDownDelay
}

// applyScaling executes a scaling decision and updates tracking state.
func (a *BasicAutoscaler) applyScaling(
	agentID string,
	pool WorkerPool,
	currentSize int,
	desiredSize int,
	pending int,
	processing int,
) {
	totalTasks := pending + processing
	utilization := float64(totalTasks) / float64(max(currentSize, 1))

	if desiredSize > currentSize {
		log.Printf("[autoscaler] %s: scaling UP %d → %d (util=%.2f, pending=%d, processing=%d)",
			agentID, currentSize, desiredSize, utilization, pending, processing)
		a.lastScaleUp[agentID] = true
	} else {
		log.Printf("[autoscaler] %s: scaling DOWN %d → %d (util=%.2f, pending=%d, processing=%d)",
			agentID, currentSize, desiredSize, utilization, pending, processing)
		a.lastScaleUp[agentID] = false
	}

	pool.SetDesiredSize(desiredSize)
	a.lastScaleTime[agentID] = time.Now()
}
