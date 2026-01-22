package telemetry

// NoOpIsolator is the default isolator for OSS.
// It passes metrics through unchanged (no tenant isolation).
type NoOpIsolator struct{}

// Isolate returns the metric unchanged (pass-through for OSS).
func (i *NoOpIsolator) Isolate(m Metric) Metric {
	return m
}

// Ensure NoOpIsolator implements MetricIsolator
var _ MetricIsolator = (*NoOpIsolator)(nil)

// LabelInjectionIsolator adds custom labels to all metrics.
// Used by cloud version to inject org_id, tenant_id, tier labels.
//
// Example usage (cloud only):
//
//	isolator := &LabelInjectionIsolator{
//	    ExtraLabels: []Label{
//	        {Key: "org_id", Value: "org-abc123"},
//	        {Key: "tenant_id", Value: "tenant-xyz789"},
//	        {Key: "tier", Value: "enterprise"},
//	    },
//	}
//	registry := NewRegistry(isolator)
type LabelInjectionIsolator struct {
	ExtraLabels []Label
}

// Isolate returns a new metric with extra labels appended.
// Original metric is not modified (immutable transformation).
func (i *LabelInjectionIsolator) Isolate(m Metric) Metric {
	// Create a copy with appended labels
	isolated := m
	isolated.Labels = make([]Label, len(m.Labels)+len(i.ExtraLabels))
	copy(isolated.Labels, m.Labels)
	copy(isolated.Labels[len(m.Labels):], i.ExtraLabels)
	return isolated
}

// Ensure LabelInjectionIsolator implements MetricIsolator
var _ MetricIsolator = (*LabelInjectionIsolator)(nil)
