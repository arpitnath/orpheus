package daemon

import (
	"context"

	"orpheus/daemon/pkg/telemetry"
)

// QueueCollector wraps PoolManager to collect request queue metrics.
// Exports metrics for: pending requests, processing requests, queue depth, max size.
type QueueCollector struct {
	poolManager   *PoolManager
	labelProvider LabelProvider // For custom per-agent labels
}

// NewQueueCollector creates a collector that wraps the pool manager's queues.
func NewQueueCollector(poolManager *PoolManager) *QueueCollector {
	return &QueueCollector{
		poolManager:   poolManager,
		labelProvider: poolManager, // PoolManager implements LabelProvider
	}
}

// Name returns the collector identifier.
func (c *QueueCollector) Name() string {
	return "queue"
}

// Collect gathers request queue metrics from all agents.
func (c *QueueCollector) Collect(ctx context.Context) ([]telemetry.Metric, error) {
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

	// Collect stats from each agent's queue
	for _, pool := range pools {
		// Get stats from queue
		stats := pool.GetQueueStats()

		// Create base labels for this agent
		baseLabels := []telemetry.Label{{Key: "agent", Value: stats.AgentID}}

		// Merge with custom labels from agent.yaml
		customLabels := c.labelProvider.GetLabelsForAgent(stats.AgentID)
		labels := MergeLabels(baseLabels, customLabels)

		// Pending requests
		metrics = append(metrics, telemetry.Metric{
			Name:        "orpheus_queue_pending",
			Type:        telemetry.MetricTypeGauge,
			Value:       float64(stats.PendingCount),
			Labels:      labels,
			Description: "Number of requests waiting in queue",
		})

		// Processing requests
		metrics = append(metrics, telemetry.Metric{
			Name:        "orpheus_queue_processing",
			Type:        telemetry.MetricTypeGauge,
			Value:       float64(stats.ProcessingCount),
			Labels:      labels,
			Description: "Number of requests currently being processed",
		})

		// Total queue depth (pending + processing)
		metrics = append(metrics, telemetry.Metric{
			Name:        "orpheus_queue_depth_total",
			Type:        telemetry.MetricTypeGauge,
			Value:       float64(stats.PendingCount + stats.ProcessingCount),
			Labels:      labels,
			Description: "Total queue depth (pending + processing)",
		})

		// Max queue size
		metrics = append(metrics, telemetry.Metric{
			Name:        "orpheus_queue_max_size",
			Type:        telemetry.MetricTypeGauge,
			Value:       float64(stats.MaxSize),
			Labels:      labels,
			Description: "Maximum queue size before rejecting requests",
		})
	}

	return metrics, nil
}

// Ensure QueueCollector implements telemetry.MetricCollector
var _ telemetry.MetricCollector = (*QueueCollector)(nil)
