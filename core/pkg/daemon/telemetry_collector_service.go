package daemon

import (
	"context"

	"orpheus/daemon/pkg/service"
	"orpheus/daemon/pkg/telemetry"
)

// ServiceCollector wraps ServiceManager to collect model server health metrics.
// Exports metrics for: service up/down status, uptime.
type ServiceCollector struct {
	serviceManager *service.Manager
}

// NewServiceCollector creates a collector that wraps the service manager.
func NewServiceCollector(serviceManager *service.Manager) *ServiceCollector {
	return &ServiceCollector{
		serviceManager: serviceManager,
	}
}

// Name returns the collector identifier.
func (c *ServiceCollector) Name() string {
	return "service"
}

// Collect gathers model server health metrics.
func (c *ServiceCollector) Collect(ctx context.Context) ([]telemetry.Metric, error) {
	// Check context before starting
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Nil safety check
	if c.serviceManager == nil {
		return []telemetry.Metric{}, nil
	}

	var metrics []telemetry.Metric

	// Get status for all servers
	servers := c.serviceManager.GetServerStatus()

	for engine, status := range servers {
		// Create labels for this engine
		labels := []telemetry.Label{{Key: "engine", Value: engine}}

		// Service up/down (1 if healthy, 0 otherwise)
		upValue := 0.0
		if status.Healthy {
			upValue = 1.0
		}
		metrics = append(metrics, telemetry.Metric{
			Name:        "orpheus_service_up",
			Type:        telemetry.MetricTypeGauge,
			Value:       upValue,
			Labels:      labels,
			Description: "Model server health (1=up, 0=down)",
		})

		// Service uptime in seconds
		if status.Uptime > 0 {
			metrics = append(metrics, telemetry.Metric{
				Name:        "orpheus_service_uptime_seconds",
				Type:        telemetry.MetricTypeGauge,
				Value:       float64(status.Uptime),
				Labels:      labels,
				Description: "Model server uptime in seconds",
			})
		}
	}

	return metrics, nil
}

// Ensure ServiceCollector implements telemetry.MetricCollector
var _ telemetry.MetricCollector = (*ServiceCollector)(nil)
