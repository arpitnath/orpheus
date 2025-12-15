// Package scaling provides interfaces and types for worker pool management
// and queue-based autoscaling of agent execution.
package scaling

import "time"

// HealthStatus represents the health state of a worker.
type HealthStatus int

const (
	// HealthUnknown indicates the worker health has not been determined.
	HealthUnknown HealthStatus = iota
	// HealthHealthy indicates the worker is functioning normally.
	HealthHealthy
	// HealthUnhealthy indicates the worker has failed or is not responding.
	HealthUnhealthy
	// HealthDegraded indicates the worker is slow or unreliable.
	HealthDegraded
)

// String returns a string representation of the health status.
func (h HealthStatus) String() string {
	switch h {
	case HealthUnknown:
		return "unknown"
	case HealthHealthy:
		return "healthy"
	case HealthUnhealthy:
		return "unhealthy"
	case HealthDegraded:
		return "degraded"
	default:
		return "unknown"
	}
}

// ScalingPolicy defines the scaling behavior for a worker pool.
type ScalingPolicy struct {
	// MinWorkers is the minimum number of workers to maintain.
	MinWorkers int

	// MaxWorkers is the maximum number of workers allowed.
	MaxWorkers int

	// TargetUtilization is the ideal ratio of tasks to workers.
	// e.g., 2.0 means we want 2 tasks per worker on average.
	TargetUtilization float64

	// ScaleUpThreshold triggers scaling up when utilization exceeds this value.
	ScaleUpThreshold float64

	// ScaleDownThreshold triggers scaling down when utilization falls below this value.
	ScaleDownThreshold float64

	// ScaleUpDelay is the minimum time between scale-up operations.
	ScaleUpDelay time.Duration

	// ScaleDownDelay is the minimum time between scale-down operations.
	ScaleDownDelay time.Duration

	// IdleTimeout is how long a worker can be idle before being terminated.
	IdleTimeout time.Duration
}

// PoolStats holds a read-only snapshot of worker pool statistics.
type PoolStats struct {
	// AgentID is the identifier of the agent this pool serves.
	AgentID string

	// TotalWorkers is the current number of workers in the pool.
	TotalWorkers int

	// IdleWorkers is the number of workers available for work.
	IdleWorkers int

	// BusyWorkers is the number of workers currently processing tasks.
	BusyWorkers int

	// DesiredSize is the target number of workers.
	DesiredSize int

	// LastScaleTime is when the last scaling operation occurred.
	LastScaleTime time.Time
}

// Result represents the output from an agent execution.
// This type is defined here to avoid circular dependencies with the proxy package.
type Result struct {
	// Status indicates the outcome: "success", "error", or "timeout".
	Status string

	// Output contains the parsed JSON output from the agent.
	Output map[string]interface{}

	// Error contains the error message if the execution failed.
	Error string

	// Stderr contains any stderr output from the agent process.
	Stderr string

	// ExitCode is the process exit code.
	ExitCode int

	// Duration is how long the execution took.
	Duration time.Duration
}
