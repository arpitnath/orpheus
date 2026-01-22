package telemetry

import "context"

// MetricCollector extracts metrics from existing system components.
// Implementations wrap existing GetStats() methods without adding new instrumentation.
//
// Example:
//
//	type PoolCollector struct {
//	    agentID string
//	    pool    scaling.WorkerPool
//	}
//
//	func (c *PoolCollector) Collect(ctx context.Context) ([]Metric, error) {
//	    stats := c.pool.GetStats()
//	    return []Metric{
//	        {Name: "orpheus_pool_workers_total", Value: float64(stats.TotalWorkers), Labels: []Label{{Key: "agent", Value: c.agentID}}},
//	    }, nil
//	}
type MetricCollector interface {
	// Name returns the collector identifier (for logging and debugging).
	Name() string

	// Collect gathers current metrics from the wrapped component.
	// Must be goroutine-safe and should complete quickly (<100ms).
	// Returns empty slice if source is unavailable (not an error).
	Collect(ctx context.Context) ([]Metric, error)
}

// MetricRegistry provides read-only access to collected metrics.
// Used by scrape endpoints to gather all current metrics.
type MetricRegistry interface {
	// Collect gathers metrics from all registered collectors.
	// Continues on individual collector failures (partial results).
	Collect(ctx context.Context) ([]Metric, error)

	// Collectors returns the names of all registered collectors.
	// Useful for debugging and testing.
	Collectors() []string
}

// MutableMetricRegistry extends MetricRegistry with collector management.
// Used during server initialization to register collectors.
type MutableMetricRegistry interface {
	MetricRegistry

	// Register adds a collector to the registry.
	// Returns error if a collector with the same name already exists.
	Register(collector MetricCollector) error

	// Unregister removes a collector by name.
	// No-op if collector doesn't exist.
	Unregister(name string)
}

// MetricExporter formats metrics for external systems.
// Implementations include PrometheusExporter (text format).
type MetricExporter interface {
	// Export formats metrics to bytes in the exporter's format.
	// Returns error if formatting fails.
	Export(metrics []Metric) ([]byte, error)

	// ContentType returns the MIME type for this export format.
	// Example: "text/plain; version=0.0.4; charset=utf-8" for Prometheus.
	ContentType() string
}

// MetricIsolator applies isolation transformations to metrics.
// OSS: NoOpIsolator (pass-through)
// Cloud: LabelInjectionIsolator (adds org_id, tenant_id, tier labels)
type MetricIsolator interface {
	// Isolate returns a new Metric with isolation applied.
	// The original metric is not modified (immutable transformation).
	Isolate(m Metric) Metric
}
