package scaling

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// BasicWorkerPool implements WorkerPool for managing a pool of workers.
// It handles worker lifecycle, scaling, and health management.
type BasicWorkerPool struct {
	agentID string
	spawner WorkerSpawner
	policy  ScalingPolicy

	workers   map[string]Worker // All workers by ID
	workersMu sync.RWMutex

	idleWorkers chan Worker // Buffered channel of available workers
	desiredSize atomic.Int32

	lastScaleTime time.Time
	scaleReason   string
	metricsMu     sync.RWMutex

	// Health tracking for auto-replacement
	replacementAttempts map[string]int
	lastReplacementTime time.Time
	replacementMu       sync.RWMutex

	// Session affinity tracking: sessionID → workerID
	sessionWorkers   map[string]string
	sessionWorkersMu sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewWorkerPool creates a new worker pool for the given agent.
// It spawns the initial workers based on the policy's MinWorkers setting.
func NewWorkerPool(agentID string, spawner WorkerSpawner, policy ScalingPolicy) *BasicWorkerPool {
	ctx, cancel := context.WithCancel(context.Background())

	p := &BasicWorkerPool{
		agentID:             agentID,
		spawner:             spawner,
		policy:              policy,
		workers:             make(map[string]Worker),
		idleWorkers:         make(chan Worker, policy.MaxWorkers),
		replacementAttempts: make(map[string]int),
		lastReplacementTime: time.Now(),
		sessionWorkers:      make(map[string]string),
		ctx:                 ctx,
		cancel:              cancel,
	}

	// Set initial desired size
	p.desiredSize.Store(int32(policy.MinWorkers))
	p.lastScaleTime = time.Now()
	p.scaleReason = "initial"

	// Spawn initial workers
	for i := 0; i < policy.MinWorkers; i++ {
		if err := p.spawnWorker(); err != nil {
			log.Printf("[pool] Failed to spawn initial worker %d for %s: %v", i+1, agentID, err)
		}
	}

	// Start maintenance loop
	p.wg.Add(1)
	go p.maintenanceLoop()

	log.Printf("[pool] Created pool for %s with %d workers (min=%d, max=%d)",
		agentID, p.Size(), policy.MinWorkers, policy.MaxWorkers)

	return p
}

// AgentID returns the identifier of the agent this pool serves.
func (p *BasicWorkerPool) AgentID() string {
	return p.agentID
}

// Size returns the current number of workers in the pool.
func (p *BasicWorkerPool) Size() int {
	p.workersMu.RLock()
	defer p.workersMu.RUnlock()
	return len(p.workers)
}

// DesiredSize returns the target number of workers.
func (p *BasicWorkerPool) DesiredSize() int {
	return int(p.desiredSize.Load())
}

// SetDesiredSize updates the target worker count.
// The pool will scale up or down to match this target during maintenance.
func (p *BasicWorkerPool) SetDesiredSize(size int) {
	// Clamp to policy bounds
	if size < p.policy.MinWorkers {
		size = p.policy.MinWorkers
	}
	if size > p.policy.MaxWorkers {
		size = p.policy.MaxWorkers
	}

	old := p.desiredSize.Swap(int32(size))
	if int(old) != size {
		p.metricsMu.Lock()
		p.lastScaleTime = time.Now()
		if size > int(old) {
			p.scaleReason = fmt.Sprintf("scale_up_%d_to_%d", old, size)
		} else {
			p.scaleReason = fmt.Sprintf("scale_down_%d_to_%d", old, size)
		}
		p.metricsMu.Unlock()

		log.Printf("[pool] %s desired size changed: %d → %d", p.agentID, old, size)
	}
}

// GetIdleWorker returns an available worker, blocking until one is ready.
// Returns an error if the context is cancelled or times out.
func (p *BasicWorkerPool) GetIdleWorker(ctx context.Context) (Worker, error) {
	select {
	case worker := <-p.idleWorkers:
		// Check worker health
		if worker.Health() == HealthUnhealthy {
			go p.removeWorker(worker.ID())
			return p.GetIdleWorker(ctx) // Retry with another worker
		}
		return worker, nil

	case <-ctx.Done():
		return nil, ctx.Err()

	case <-time.After(30 * time.Second):
		// Emergency spawn if under max
		if p.Size() < p.policy.MaxWorkers {
			log.Printf("[pool] %s: emergency spawn after 30s timeout", p.agentID)
			if err := p.spawnWorker(); err == nil {
				// Try to get the new worker
				select {
				case worker := <-p.idleWorkers:
					return worker, nil
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(5 * time.Second):
					return nil, fmt.Errorf("no workers available after emergency spawn")
				}
			}
		}
		return nil, fmt.Errorf("no workers available for %s (size=%d, max=%d)",
			p.agentID, p.Size(), p.policy.MaxWorkers)
	}
}

// ReturnWorker returns a worker to the pool after use.
// The worker is health-checked before being made available again.
func (p *BasicWorkerPool) ReturnWorker(worker Worker) {
	if worker == nil {
		return
	}

	// Verify worker belongs to pool
	p.workersMu.RLock()
	_, exists := p.workers[worker.ID()]
	p.workersMu.RUnlock()

	if !exists {
		log.Printf("[pool] %s: attempted to return unknown worker %s", p.agentID, worker.ID())
		return
	}

	// Health check before returning
	if worker.Health() == HealthUnhealthy {
		log.Printf("[pool] %s: removing unhealthy worker %s", p.agentID, worker.ID())
		go p.removeWorker(worker.ID())
		return
	}

	// Return to idle pool (non-blocking)
	select {
	case p.idleWorkers <- worker:
		// Successfully returned
	default:
		// Channel full - should not happen in normal operation
		log.Printf("[pool] %s: idle channel full, dropping worker %s", p.agentID, worker.ID())
	}
}

// AcquireForSession tries to get the worker that previously handled this session.
// Falls back to any idle worker if preferred worker unavailable within maxWait.
// If sessionID is empty, behaves like GetIdleWorker (backward compatible).
func (p *BasicWorkerPool) AcquireForSession(ctx context.Context, sessionID string, maxWait time.Duration) (Worker, error) {
	// No session = any worker
	if sessionID == "" {
		return p.GetIdleWorker(ctx)
	}

	// Check if session has a preferred worker
	p.sessionWorkersMu.RLock()
	preferredWorkerID, hasPreferred := p.sessionWorkers[sessionID]
	p.sessionWorkersMu.RUnlock()

	if !hasPreferred {
		// First request for this session - get any worker
		worker, err := p.GetIdleWorker(ctx)
		if err != nil {
			return nil, err
		}
		// Track session → worker mapping
		p.sessionWorkersMu.Lock()
		p.sessionWorkers[sessionID] = worker.ID()
		p.sessionWorkersMu.Unlock()
		log.Printf("[pool] %s: session %s assigned to worker %s", p.agentID, sessionID, worker.ID())
		return worker, nil
	}

	// Try to get preferred worker from idle channel
	worker := p.tryGetSpecificWorker(preferredWorkerID)
	if worker != nil {
		log.Printf("[pool] %s: session %s got preferred worker %s", p.agentID, sessionID, worker.ID())
		return worker, nil
	}

	// Preferred worker busy - wait briefly
	if maxWait > 0 {
		worker = p.waitForSpecificWorker(ctx, preferredWorkerID, maxWait)
		if worker != nil {
			log.Printf("[pool] %s: session %s got preferred worker %s after wait", p.agentID, sessionID, worker.ID())
			return worker, nil
		}
	}

	// Fallback to any idle worker
	log.Printf("[pool] %s: session %s fallback to any worker (preferred %s busy)", p.agentID, sessionID, preferredWorkerID)
	worker, err := p.GetIdleWorker(ctx)
	if err != nil {
		return nil, err
	}
	// Update session mapping to new worker
	p.sessionWorkersMu.Lock()
	p.sessionWorkers[sessionID] = worker.ID()
	p.sessionWorkersMu.Unlock()
	return worker, nil
}

// tryGetSpecificWorker attempts to extract a specific worker from the idle channel.
// Returns nil if worker not in channel (busy or doesn't exist).
func (p *BasicWorkerPool) tryGetSpecificWorker(workerID string) Worker {
	// Drain and re-queue workers until we find the one we want or exhaust options
	var found Worker
	checked := make([]Worker, 0)

	for {
		select {
		case w := <-p.idleWorkers:
			if w.ID() == workerID {
				found = w
				goto done
			}
			checked = append(checked, w)
		default:
			// Channel empty
			goto done
		}
	}
done:
	// Return non-matching workers to channel
	for _, w := range checked {
		select {
		case p.idleWorkers <- w:
		default:
			// Channel full - shouldn't happen
		}
	}
	return found
}

// waitForSpecificWorker waits up to maxWait for a specific worker to become idle.
func (p *BasicWorkerPool) waitForSpecificWorker(ctx context.Context, workerID string, maxWait time.Duration) Worker {
	deadline := time.Now().Add(maxWait)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for time.Now().Before(deadline) {
		if worker := p.tryGetSpecificWorker(workerID); worker != nil {
			return worker
		}
		select {
		case <-ticker.C:
			continue
		case <-ctx.Done():
			return nil
		}
	}
	return nil
}

// ClearSession removes session tracking for a session ID.
func (p *BasicWorkerPool) ClearSession(sessionID string) {
	p.sessionWorkersMu.Lock()
	delete(p.sessionWorkers, sessionID)
	p.sessionWorkersMu.Unlock()
}

// GetSessionWorker returns the worker ID associated with a session, if any.
func (p *BasicWorkerPool) GetSessionWorker(sessionID string) (string, bool) {
	p.sessionWorkersMu.RLock()
	defer p.sessionWorkersMu.RUnlock()
	workerID, ok := p.sessionWorkers[sessionID]
	return workerID, ok
}

// GetStats returns a snapshot of the pool's current statistics.
func (p *BasicWorkerPool) GetStats() PoolStats {
	p.workersMu.RLock()
	totalWorkers := len(p.workers)
	p.workersMu.RUnlock()

	idleWorkers := len(p.idleWorkers)
	busyWorkers := totalWorkers - idleWorkers

	p.metricsMu.RLock()
	lastScaleTime := p.lastScaleTime
	p.metricsMu.RUnlock()

	return PoolStats{
		AgentID:       p.agentID,
		TotalWorkers:  totalWorkers,
		IdleWorkers:   idleWorkers,
		BusyWorkers:   busyWorkers,
		DesiredSize:   p.DesiredSize(),
		LastScaleTime: lastScaleTime,
	}
}

// Shutdown gracefully stops all workers and closes the pool.
func (p *BasicWorkerPool) Shutdown(ctx context.Context) error {
	log.Printf("[pool] %s: shutting down...", p.agentID)

	// Stop maintenance loop
	p.cancel()
	p.wg.Wait()

	// Shutdown all workers
	p.workersMu.Lock()
	workers := make([]Worker, 0, len(p.workers))
	for _, w := range p.workers {
		workers = append(workers, w)
	}
	p.workersMu.Unlock()

	var wg sync.WaitGroup
	for _, w := range workers {
		wg.Add(1)
		go func(worker Worker) {
			defer wg.Done()
			if err := worker.Shutdown(ctx); err != nil {
				log.Printf("[pool] %s: error shutting down worker %s: %v",
					p.agentID, worker.ID(), err)
			}
		}(w)
	}

	// Wait for all workers to shutdown or context to cancel
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Printf("[pool] %s: all workers shut down", p.agentID)
	case <-ctx.Done():
		log.Printf("[pool] %s: shutdown timed out", p.agentID)
		return ctx.Err()
	}

	// Clear the workers map
	p.workersMu.Lock()
	p.workers = make(map[string]Worker)
	p.workersMu.Unlock()

	// Drain idle channel
	close(p.idleWorkers)
	for range p.idleWorkers {
		// Drain
	}

	return nil
}

// spawnWorker creates a new worker and adds it to the pool.
func (p *BasicWorkerPool) spawnWorker() error {
	worker, err := p.spawner.SpawnWorker(p.ctx, p.agentID)
	if err != nil {
		return fmt.Errorf("failed to spawn worker: %w", err)
	}

	p.workersMu.Lock()
	p.workers[worker.ID()] = worker
	p.workersMu.Unlock()

	// Add to idle pool
	select {
	case p.idleWorkers <- worker:
		log.Printf("[pool] %s: spawned worker %s (total=%d)", p.agentID, worker.ID(), p.Size())
	default:
		// Channel full - remove the worker
		p.workersMu.Lock()
		delete(p.workers, worker.ID())
		p.workersMu.Unlock()
		return fmt.Errorf("idle channel full, cannot add worker")
	}

	return nil
}

// removeWorker removes a worker from the pool.
func (p *BasicWorkerPool) removeWorker(workerID string) error {
	p.workersMu.Lock()
	worker, exists := p.workers[workerID]
	if !exists {
		p.workersMu.Unlock()
		return fmt.Errorf("worker %s not found", workerID)
	}
	delete(p.workers, workerID)
	p.workersMu.Unlock()

	// Shutdown the worker
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := worker.Shutdown(ctx); err != nil {
		log.Printf("[pool] %s: error shutting down worker %s: %v", p.agentID, workerID, err)
	}

	log.Printf("[pool] %s: removed worker %s (total=%d)", p.agentID, workerID, p.Size())

	// Spawn replacement if below minimum
	if p.Size() < p.policy.MinWorkers {
		go p.spawnWorker()
	}

	return nil
}

// maintenanceLoop periodically performs pool maintenance.
func (p *BasicWorkerPool) maintenanceLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.performMaintenance()
		}
	}
}

// performMaintenance handles scaling and cleanup operations.
func (p *BasicWorkerPool) performMaintenance() {
	current := p.Size()
	desired := p.DesiredSize()

	// Scale up if needed
	if current < desired {
		toSpawn := desired - current
		log.Printf("[pool] %s: scaling up %d → %d (spawning %d workers)",
			p.agentID, current, desired, toSpawn)

		for i := 0; i < toSpawn; i++ {
			if err := p.spawnWorker(); err != nil {
				log.Printf("[pool] %s: failed to spawn worker during scale-up: %v", p.agentID, err)
				break
			}
		}
	}

	// Scale down if needed - remove idle workers immediately when autoscaler decides
	// Note: Autoscaler already has ScaleDownDelay to prevent thrashing, so we respect its decision
	if current > desired {
		toRemove := current - desired
		log.Printf("[pool] %s: scaling down %d → %d (removing up to %d workers)",
			p.agentID, current, desired, toRemove)

		removed := 0
	scaleDownLoop:
		for i := 0; i < toRemove; i++ {
			select {
			case worker := <-p.idleWorkers:
				// Remove idle worker immediately - autoscaler's ScaleDownDelay already prevents thrashing
				p.removeWorker(worker.ID())
				removed++
			default:
				// No more idle workers available to remove
				break scaleDownLoop
			}
		}

		if removed > 0 {
			log.Printf("[pool] %s: removed %d idle workers", p.agentID, removed)
		}
	}

	// Health check all workers
	p.performHealthCheck()
}

// performHealthCheck checks the health of all workers and triggers replacement for unhealthy ones.
func (p *BasicWorkerPool) performHealthCheck() {
	p.workersMu.RLock()
	workers := make([]Worker, 0, len(p.workers))
	for _, w := range p.workers {
		workers = append(workers, w)
	}
	p.workersMu.RUnlock()

	for _, worker := range workers {
		if worker.Health() == HealthUnhealthy {
			go p.replaceUnhealthyWorker(worker)
		}
	}
}

// replaceUnhealthyWorker removes an unhealthy worker and spawns a replacement.
// It includes rate limiting and max attempt tracking to prevent replacement storms.
func (p *BasicWorkerPool) replaceUnhealthyWorker(worker Worker) {
	workerID := worker.ID()

	// Rate limiting: prevent replacement storms with 5s global cooldown
	p.replacementMu.Lock()
	if time.Since(p.lastReplacementTime) < 5*time.Second {
		p.replacementMu.Unlock()
		log.Printf("[pool] %s: rate limiting replacement for %s", p.agentID, workerID)
		return
	}
	p.lastReplacementTime = time.Now()

	// Track replacement attempts per worker
	attempts := p.replacementAttempts[workerID]
	if attempts >= 3 {
		// Max attempts reached - give up and clean up tracking
		delete(p.replacementAttempts, workerID)
		p.replacementMu.Unlock()
		log.Printf("[pool] %s: max replacement attempts (3) reached for %s, giving up", p.agentID, workerID)
		return
	}
	p.replacementAttempts[workerID] = attempts + 1
	p.replacementMu.Unlock()

	log.Printf("[pool] %s: replacing unhealthy worker %s (attempt %d/3)", p.agentID, workerID, attempts+1)

	// Remove the unhealthy worker
	if err := p.removeWorker(workerID); err != nil {
		log.Printf("[pool] %s: failed to remove unhealthy worker %s: %v", p.agentID, workerID, err)
		return
	}

	// Check if we should spawn a replacement
	if p.Size() < p.policy.MinWorkers || p.Size() < p.DesiredSize() {
		// Spawn replacement with linear backoff
		for attempt := 1; attempt <= 3; attempt++ {
			err := p.spawnWorker()
			if err == nil {
				// Success - clear attempt counter
				p.replacementMu.Lock()
				delete(p.replacementAttempts, workerID)
				p.replacementMu.Unlock()
				log.Printf("[pool] %s: successfully spawned replacement worker", p.agentID)
				return
			}

			log.Printf("[pool] %s: failed to spawn replacement (attempt %d/3): %v", p.agentID, attempt, err)

			// Linear backoff before retry
			if attempt < 3 {
				backoff := time.Duration(attempt) * time.Second
				time.Sleep(backoff)
			}
		}

		log.Printf("[pool] %s: failed to spawn replacement after 3 attempts", p.agentID)
	} else {
		log.Printf("[pool] %s: no replacement needed (size=%d, min=%d, desired=%d)",
			p.agentID, p.Size(), p.policy.MinWorkers, p.DesiredSize())
	}
}
