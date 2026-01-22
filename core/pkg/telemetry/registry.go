package telemetry

import (
	"context"
	"fmt"
	"log"
	"sync"
)

// DefaultRegistry implements MutableMetricRegistry with thread-safe collector management.
type DefaultRegistry struct {
	collectors map[string]MetricCollector
	isolator   MetricIsolator
	mu         sync.RWMutex
}

// NewRegistry creates a new metric registry with the given isolator.
// If isolator is nil, uses NoOpIsolator (pass-through for OSS).
func NewRegistry(isolator MetricIsolator) *DefaultRegistry {
	if isolator == nil {
		isolator = &NoOpIsolator{}
	}

	return &DefaultRegistry{
		collectors: make(map[string]MetricCollector),
		isolator:   isolator,
	}
}

// Register adds a collector to the registry.
// Returns error if a collector with the same name already exists.
func (r *DefaultRegistry) Register(collector MetricCollector) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := collector.Name()
	if _, exists := r.collectors[name]; exists {
		return fmt.Errorf("collector already registered: %s", name)
	}

	r.collectors[name] = collector
	log.Printf("[telemetry] Registered collector: %s", name)
	return nil
}

// Unregister removes a collector by name.
// No-op if collector doesn't exist.
func (r *DefaultRegistry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.collectors[name]; exists {
		delete(r.collectors, name)
		log.Printf("[telemetry] Unregistered collector: %s", name)
	}
}

// Collectors returns the names of all registered collectors.
func (r *DefaultRegistry) Collectors() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.collectors))
	for name := range r.collectors {
		names = append(names, name)
	}
	return names
}

// Collect gathers metrics from all registered collectors.
// Continues on individual collector failures (partial results).
func (r *DefaultRegistry) Collect(ctx context.Context) ([]Metric, error) {
	// Copy collectors list while holding read lock
	r.mu.RLock()
	collectors := make([]MetricCollector, 0, len(r.collectors))
	for _, collector := range r.collectors {
		collectors = append(collectors, collector)
	}
	r.mu.RUnlock()

	// Collect from each collector without holding lock
	var allMetrics []Metric
	var collectionErrors []error

	for _, collector := range collectors {
		// Check context cancellation before each collector
		if err := ctx.Err(); err != nil {
			return allMetrics, fmt.Errorf("collection cancelled: %w", err)
		}

		// Safely collect with panic recovery
		metrics, err := safeCollect(collector, ctx)
		if err != nil {
			// Log error but continue with other collectors (partial results)
			errMsg := fmt.Errorf("collector %s failed: %w", collector.Name(), err)
			collectionErrors = append(collectionErrors, errMsg)
			log.Printf("[telemetry] %v", errMsg)
			continue
		}

		// Apply isolation to each metric
		for _, metric := range metrics {
			isolated := r.isolator.Isolate(metric)
			allMetrics = append(allMetrics, isolated)
		}
	}

	// Return partial results with combined error
	if len(collectionErrors) > 0 {
		// Return metrics with error indicating partial failure
		return allMetrics, fmt.Errorf("%d collector(s) failed (partial results returned)", len(collectionErrors))
	}

	return allMetrics, nil
}

// safeCollect calls collector.Collect() with panic recovery.
// Prevents one panicking collector from crashing the entire scrape.
func safeCollect(collector MetricCollector, ctx context.Context) (metrics []Metric, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("collector %s panicked: %v", collector.Name(), r)
			metrics = nil
		}
	}()
	return collector.Collect(ctx)
}

// Ensure DefaultRegistry implements MutableMetricRegistry
var _ MutableMetricRegistry = (*DefaultRegistry)(nil)
