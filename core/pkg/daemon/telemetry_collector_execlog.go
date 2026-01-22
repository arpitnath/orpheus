package daemon

import (
	"context"
	"log"

	"orpheus/daemon/pkg/execlog"
	"orpheus/daemon/pkg/registry"
	"orpheus/daemon/pkg/telemetry"
)

// ExecLogCollector wraps ExecLog to collect execution metrics.
// Exports metrics for: total requests, requests by status, success rate.
type ExecLogCollector struct {
	registry      registry.Registry
	execlogDir    string
	labelProvider LabelProvider // For custom per-agent labels
}

// NewExecLogCollector creates a collector that wraps ExecLog readers.
func NewExecLogCollector(registry registry.Registry, execlogDir string, labelProvider LabelProvider) *ExecLogCollector {
	return &ExecLogCollector{
		registry:      registry,
		execlogDir:    execlogDir,
		labelProvider: labelProvider,
	}
}

// Name returns the collector identifier.
func (c *ExecLogCollector) Name() string {
	return "execlog"
}

// Collect gathers execution metrics from all agents' ExecLogs.
func (c *ExecLogCollector) Collect(ctx context.Context) ([]telemetry.Metric, error) {
	// Check context before starting
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Get all agents from registry
	agents, err := c.registry.List()
	if err != nil {
		log.Printf("[telemetry] Failed to list agents for execlog: %v", err)
		return nil, err
	}

	var metrics []telemetry.Metric

	// Collect ExecLog stats for each agent
	for _, agent := range agents {
		// Use closure to ensure defer runs per-iteration (proper cleanup on panic)
		agentMetrics := func(agentName string) []telemetry.Metric {
			// Create ExecLog reader for this agent
			reader, err := execlog.NewReader(c.execlogDir, agentName)
			if err != nil {
				// No execlog for this agent (no executions yet)
				return nil
			}
			defer reader.Close()

			// Get stats
			stats, err := reader.GetStats()
			if err != nil {
				log.Printf("[telemetry] Failed to get execlog stats for %s: %v", agentName, err)
				return nil
			}

			// Create base labels for this agent
			baseLabels := []telemetry.Label{{Key: "agent", Value: agentName}}

			// Merge with custom labels from agent.yaml
			var customLabels []telemetry.Label
			if c.labelProvider != nil {
				customLabels = c.labelProvider.GetLabelsForAgent(agentName)
			}
			labels := MergeLabels(baseLabels, customLabels)
			var result []telemetry.Metric

			// Total requests
			result = append(result, telemetry.Metric{
				Name:        "orpheus_execlog_requests_total",
				Type:        telemetry.MetricTypeCounter,
				Value:       float64(stats.Total),
				Labels:      labels,
				Description: "Total number of execution requests",
			})

			// Requests by status (completed)
			completedLabels := append([]telemetry.Label(nil), labels...)
			completedLabels = append(completedLabels, telemetry.Label{Key: "status", Value: "completed"})
			result = append(result, telemetry.Metric{
				Name:        "orpheus_execlog_requests_by_status",
				Type:        telemetry.MetricTypeCounter,
				Value:       float64(stats.Completed),
				Labels:      completedLabels,
				Description: "Number of requests by terminal status",
			})

			// Requests by status (failed)
			failedLabels := append([]telemetry.Label(nil), labels...)
			failedLabels = append(failedLabels, telemetry.Label{Key: "status", Value: "failed"})
			result = append(result, telemetry.Metric{
				Name:        "orpheus_execlog_requests_by_status",
				Type:        telemetry.MetricTypeCounter,
				Value:       float64(stats.Failed),
				Labels:      failedLabels,
				Description: "Number of requests by terminal status",
			})

			// Requests by status (crashed)
			crashedLabels := append([]telemetry.Label(nil), labels...)
			crashedLabels = append(crashedLabels, telemetry.Label{Key: "status", Value: "crashed"})
			result = append(result, telemetry.Metric{
				Name:        "orpheus_execlog_requests_by_status",
				Type:        telemetry.MetricTypeCounter,
				Value:       float64(stats.Crashed),
				Labels:      crashedLabels,
				Description: "Number of requests by terminal status",
			})

			// Success rate
			result = append(result, telemetry.Metric{
				Name:        "orpheus_execlog_success_rate",
				Type:        telemetry.MetricTypeGauge,
				Value:       stats.SuccessRate,
				Labels:      labels,
				Description: "Success rate percentage (0-100)",
			})

			// Average duration (convert from milliseconds to seconds)
			if stats.Completed > 0 {
				result = append(result, telemetry.Metric{
					Name:        "orpheus_execlog_avg_duration_seconds",
					Type:        telemetry.MetricTypeGauge,
					Value:       stats.AvgDuration / 1000.0,
					Labels:      labels,
					Description: "Average request execution duration in seconds",
				})
			}

			return result
		}(agent.Name)

		metrics = append(metrics, agentMetrics...)
	}

	return metrics, nil
}

// Ensure ExecLogCollector implements telemetry.MetricCollector
var _ telemetry.MetricCollector = (*ExecLogCollector)(nil)
