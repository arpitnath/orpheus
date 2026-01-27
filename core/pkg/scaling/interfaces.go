package scaling

import (
	"context"
	"time"
)

// Worker represents an individual execution unit that can process tasks.
type Worker interface {
	// ID returns the unique identifier for this worker.
	ID() string

	// AgentID returns the identifier of the agent this worker serves.
	AgentID() string

	// IsIdle returns true if the worker is available for work.
	IsIdle() bool

	// LastUsed returns when the worker was last used for execution.
	// Used for idle timeout tracking.
	LastUsed() time.Time

	// Execute runs a task with the given input and returns the result.
	// The input is JSON bytes that will be passed to the agent.
	Execute(ctx context.Context, input []byte) (*Result, error)

	// Shutdown gracefully stops the worker.
	Shutdown(ctx context.Context) error

	// Health returns the current health status of the worker.
	Health() HealthStatus
}

// WorkerSpawner creates and destroys workers.
// Implementations handle the specifics of how workers are created
// (e.g., using Runner, containers, VMs).
type WorkerSpawner interface {
	// SpawnWorker creates a new worker for the given agent.
	SpawnWorker(ctx context.Context, agentID string) (Worker, error)

	// KillWorker terminates a worker by its ID.
	KillWorker(ctx context.Context, workerID string) error
}

// WorkerPool manages a pool of workers for a specific agent.
// It handles worker lifecycle, scaling, and work distribution.
type WorkerPool interface {
	// AgentID returns the identifier of the agent this pool serves.
	AgentID() string

	// Size returns the current number of workers in the pool.
	Size() int

	// DesiredSize returns the target number of workers.
	DesiredSize() int

	// SetDesiredSize updates the target worker count.
	// The pool will scale up or down to match this target.
	SetDesiredSize(size int)

	// ScaleNow triggers immediate reactive scaling to the target size.
	// Unlike SetDesiredSize which waits for the maintenance loop, this sends
	// a signal to trigger scaling immediately (sub-second response).
	ScaleNow(size int)

	// GetIdleWorker returns an available worker, blocking until one is ready.
	// Returns an error if the context is cancelled or times out.
	GetIdleWorker(ctx context.Context) (Worker, error)

	// ReturnWorker returns a worker to the pool after use.
	// The worker should be health-checked before being made available again.
	ReturnWorker(worker Worker)

	// GetStats returns a snapshot of the pool's current statistics.
	GetStats() PoolStats

	// Shutdown gracefully stops all workers and closes the pool.
	Shutdown(ctx context.Context) error
}

// Autoscaler monitors queue metrics and adjusts worker pool sizes.
// It implements the queue-based scaling algorithm that is AgentScale's
// key differentiator from traditional CPU-based autoscaling.
type Autoscaler interface {
	// Start begins the scaling loop that periodically evaluates pools.
	Start(ctx context.Context) error

	// Stop halts the scaling loop.
	Stop() error

	// RegisterPool adds a worker pool to be managed by the autoscaler.
	RegisterPool(agentID string, pool WorkerPool, policy ScalingPolicy)

	// UnregisterPool removes a pool from autoscaler management.
	UnregisterPool(agentID string)

	// RegisterQueueMetrics provides queue depth information for scaling decisions.
	RegisterQueueMetrics(agentID string, metrics QueueMetrics)
}

// QueueMetrics provides queue depth information used by the autoscaler
// to make scaling decisions. This is the input to the scaling algorithm:
//
//	utilization = (PendingTasks + ProcessingTasks) / workers
type QueueMetrics interface {
	// PendingTasks returns the number of tasks waiting in the queue.
	PendingTasks() int

	// ProcessingTasks returns the number of tasks currently being processed.
	ProcessingTasks() int

	// QueueLength returns the total number of tasks (pending + processing).
	QueueLength() int
}
