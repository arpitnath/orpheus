package daemon

import (
	"context"

	"orpheus/daemon/pkg/telemetry"
)

// PoolCollector wraps PoolManager to collect worker pool metrics.
// Exports metrics for: total workers, idle workers, busy workers, desired size.
type PoolCollector struct {
	poolManager   *PoolManager
	labelProvider LabelProvider // For custom per-agent labels
}

// NewPoolCollector creates a collector that wraps the pool manager.
func NewPoolCollector(poolManager *PoolManager) *PoolCollector {
	return &PoolCollector{
		poolManager:   poolManager,
		labelProvider: poolManager, // PoolManager implements LabelProvider
	}
}

// Name returns the collector identifier.
func (c *PoolCollector) Name() string {
	return "pool"
}

// Collect gathers worker pool metrics from all agents.
func (c *PoolCollector) Collect(ctx context.Context) ([]telemetry.Metric, error) {
	// Check context before starting
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Nil safety check
	if c.poolManager == nil {
		return []telemetry.Metric{}, nil
	}

	// Get all agent pools
	pools := c.poolManager.GetAllPools()

	var metrics []telemetry.Metric

	// Collect stats from each agent's pool
	for _, pool := range pools {
		// Get stats from pool
		stats := pool.GetPoolStats()

		// Create base labels for this agent
		baseLabels := []telemetry.Label{{Key: "agent", Value: stats.AgentID}}

		// Merge with custom labels from agent.yaml
		customLabels := c.labelProvider.GetLabelsForAgent(stats.AgentID)
		labels := MergeLabels(baseLabels, customLabels)

		// Total workers
		metrics = append(metrics, telemetry.Metric{
			Name:        "orpheus_pool_workers_total",
			Type:        telemetry.MetricTypeGauge,
			Value:       float64(stats.TotalWorkers),
			Labels:      labels,
			Description: "Total number of workers in the pool",
		})

		// Idle workers
		metrics = append(metrics, telemetry.Metric{
			Name:        "orpheus_pool_workers_idle",
			Type:        telemetry.MetricTypeGauge,
			Value:       float64(stats.IdleWorkers),
			Labels:      labels,
			Description: "Number of idle workers available for work",
		})

		// Busy workers
		metrics = append(metrics, telemetry.Metric{
			Name:        "orpheus_pool_workers_busy",
			Type:        telemetry.MetricTypeGauge,
			Value:       float64(stats.BusyWorkers),
			Labels:      labels,
			Description: "Number of workers currently processing requests",
		})

		// Desired size (from autoscaler)
		metrics = append(metrics, telemetry.Metric{
			Name:        "orpheus_pool_desired_size",
			Type:        telemetry.MetricTypeGauge,
			Value:       float64(stats.DesiredSize),
			Labels:      labels,
			Description: "Target number of workers as determined by autoscaler",
		})
	}

	return metrics, nil
}

// Ensure PoolCollector implements telemetry.MetricCollector
var _ telemetry.MetricCollector = (*PoolCollector)(nil)
