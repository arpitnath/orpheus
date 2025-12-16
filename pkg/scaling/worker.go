package scaling

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"agentscale/pkg/config"
	"agentscale/pkg/runner"
)

// AgentWorker implements the Worker interface by wrapping an existing Runner.
// Each worker maintains its own Runner instance for executing agent tasks.
type AgentWorker struct {
	id      string
	agentID string
	cfg     *config.AgentConfig
	rnr     *runner.Runner

	lastUsed            atomic.Int64 // Unix nano timestamp
	idle                atomic.Bool  // Currently idle
	health              atomic.Int32 // HealthStatus as int32
	consecutiveFailures atomic.Int32 // Track failures for degradation

	mu       sync.Mutex // Protects shutdown state
	shutdown bool
}

// newAgentWorker creates a new AgentWorker with the given configuration.
func newAgentWorker(id, agentID string, cfg *config.AgentConfig) (*AgentWorker, error) {
	rnr := runner.New(cfg)

	w := &AgentWorker{
		id:      id,
		agentID: agentID,
		cfg:     cfg,
		rnr:     rnr,
	}

	// Initialize state
	w.idle.Store(true)
	w.health.Store(int32(HealthHealthy))
	w.lastUsed.Store(time.Now().UnixNano())

	return w, nil
}

// ID returns the unique identifier for this worker.
func (w *AgentWorker) ID() string {
	return w.id
}

// AgentID returns the identifier of the agent this worker serves.
func (w *AgentWorker) AgentID() string {
	return w.agentID
}

// IsIdle returns true if the worker is available for work.
func (w *AgentWorker) IsIdle() bool {
	return w.idle.Load()
}

// LastUsed returns when the worker was last used for execution.
func (w *AgentWorker) LastUsed() time.Time {
	return time.Unix(0, w.lastUsed.Load())
}

// Health returns the current health status of the worker.
func (w *AgentWorker) Health() HealthStatus {
	return HealthStatus(w.health.Load())
}

// Execute runs a task with the given input and returns the result.
// The input is JSON bytes that will be passed to the agent.
func (w *AgentWorker) Execute(ctx context.Context, input []byte) (*Result, error) {
	w.mu.Lock()
	if w.shutdown {
		w.mu.Unlock()
		return nil, fmt.Errorf("worker %s is shut down", w.id)
	}
	w.mu.Unlock()

	// Mark as busy
	w.idle.Store(false)
	defer func() {
		w.idle.Store(true)
		w.lastUsed.Store(time.Now().UnixNano())
	}()

	// Run via existing Runner
	proxyResult, err := w.rnr.Run(ctx, &runner.RunOptions{
		Input: string(input), // []byte → string conversion
	})

	if err != nil {
		// Track consecutive failures for degradation
		failures := w.consecutiveFailures.Add(1)
		if failures >= 3 {
			// After 3 consecutive failures, mark as unhealthy
			w.health.Store(int32(HealthUnhealthy))
		} else {
			// First or second failure - mark as degraded
			w.health.Store(int32(HealthDegraded))
		}
		return nil, err
	}

	// Check for error status in result
	if proxyResult.Status == "error" || proxyResult.Status == "timeout" {
		// Track failures for non-fatal errors
		failures := w.consecutiveFailures.Add(1)
		if failures >= 3 {
			w.health.Store(int32(HealthUnhealthy))
		} else {
			w.health.Store(int32(HealthDegraded))
		}
	} else {
		// Success - reset failure counter and mark healthy
		w.consecutiveFailures.Store(0)
		w.health.Store(int32(HealthHealthy))
	}

	// Convert proxy.Result → scaling.Result
	return &Result{
		Status:   string(proxyResult.Status),
		Output:   proxyResult.Output,
		Error:    proxyResult.Error,
		Stderr:   proxyResult.Stderr,
		ExitCode: proxyResult.ExitCode,
		Duration: proxyResult.Duration,
	}, nil
}

// Shutdown gracefully stops the worker.
func (w *AgentWorker) Shutdown(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.shutdown {
		return nil // Already shut down
	}

	w.shutdown = true
	w.health.Store(int32(HealthUnhealthy))

	// The Runner doesn't have explicit cleanup, but we mark the worker as done
	return nil
}

// AgentSpawner implements WorkerSpawner for creating AgentWorker instances.
type AgentSpawner struct {
	cfg     *config.AgentConfig
	agentID string
	counter atomic.Int64 // For unique worker IDs
}

// NewAgentSpawner creates a new spawner for the given agent configuration.
func NewAgentSpawner(cfg *config.AgentConfig) *AgentSpawner {
	return &AgentSpawner{
		cfg:     cfg,
		agentID: cfg.Name,
	}
}

// SpawnWorker creates a new worker for the given agent.
func (s *AgentSpawner) SpawnWorker(ctx context.Context, agentID string) (Worker, error) {
	// Generate unique worker ID
	count := s.counter.Add(1)
	workerID := fmt.Sprintf("%s-worker-%d", agentID, count)

	// Create the worker
	worker, err := newAgentWorker(workerID, agentID, s.cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create worker %s: %w", workerID, err)
	}

	return worker, nil
}

// KillWorker terminates a worker by its ID.
// Note: Since workers are in-process, this just marks them for removal.
// The actual cleanup happens when the worker is removed from the pool.
func (s *AgentSpawner) KillWorker(ctx context.Context, workerID string) error {
	// Workers are managed by the pool, so we don't track them here.
	// This is a no-op for in-process workers.
	return nil
}
