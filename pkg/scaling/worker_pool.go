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

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewWorkerPool creates a new worker pool for the given agent.
// It spawns the initial workers based on the policy's MinWorkers setting.
func NewWorkerPool(agentID string, spawner WorkerSpawner, policy ScalingPolicy) *BasicWorkerPool {
	ctx, cancel := context.WithCancel(context.Background())

	p := &BasicWorkerPool{
		agentID:     agentID,
		spawner:     spawner,
		policy:      policy,
		workers:     make(map[string]Worker),
		idleWorkers: make(chan Worker, policy.MaxWorkers),
		ctx:         ctx,
		cancel:      cancel,
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

	// Scale down if needed (only remove idle workers past timeout)
	if current > desired {
		toRemove := current - desired
		log.Printf("[pool] %s: scaling down %d → %d (removing up to %d workers)",
			p.agentID, current, desired, toRemove)

		removed := 0
		for i := 0; i < toRemove; i++ {
			select {
			case worker := <-p.idleWorkers:
				if time.Since(worker.LastUsed()) > p.policy.IdleTimeout {
					p.removeWorker(worker.ID())
					removed++
				} else {
					// Return if not idle long enough
					select {
					case p.idleWorkers <- worker:
					default:
					}
				}
			default:
				// No more idle workers to check
				break
			}
		}

		if removed > 0 {
			log.Printf("[pool] %s: removed %d idle workers", p.agentID, removed)
		}
	}
}
