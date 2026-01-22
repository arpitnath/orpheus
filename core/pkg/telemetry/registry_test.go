package telemetry

import (
	"context"
	"errors"
	"testing"
	"time"
)

// mockCollector is a test double for MetricCollector
type mockCollector struct {
	name    string
	metrics []Metric
	err     error
}

func (m *mockCollector) Name() string {
	return m.name
}

func (m *mockCollector) Collect(ctx context.Context) ([]Metric, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.metrics, nil
}

func TestNewRegistry(t *testing.T) {
	// Test with nil isolator (should default to NoOpIsolator)
	reg := NewRegistry(nil)
	if reg == nil {
		t.Fatal("NewRegistry returned nil")
	}
	if reg.isolator == nil {
		t.Error("Expected default isolator, got nil")
	}
	if len(reg.collectors) != 0 {
		t.Errorf("Expected empty collectors map, got %d collectors", len(reg.collectors))
	}

	// Test with custom isolator
	customIsolator := &NoOpIsolator{}
	reg = NewRegistry(customIsolator)
	if reg.isolator != customIsolator {
		t.Error("Expected custom isolator to be set")
	}
}

func TestRegister(t *testing.T) {
	reg := NewRegistry(nil)

	// Test successful registration
	collector := &mockCollector{name: "test"}
	err := reg.Register(collector)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Verify collector was added
	collectors := reg.Collectors()
	if len(collectors) != 1 {
		t.Errorf("Expected 1 collector, got %d", len(collectors))
	}
	if collectors[0] != "test" {
		t.Errorf("Expected collector name 'test', got '%s'", collectors[0])
	}

	// Test duplicate registration (should error)
	err = reg.Register(collector)
	if err == nil {
		t.Error("Expected error on duplicate registration, got nil")
	}
}

func TestUnregister(t *testing.T) {
	reg := NewRegistry(nil)
	collector := &mockCollector{name: "test"}
	reg.Register(collector)

	// Verify registered
	if len(reg.Collectors()) != 1 {
		t.Error("Collector not registered")
	}

	// Unregister
	reg.Unregister("test")
	if len(reg.Collectors()) != 0 {
		t.Error("Collector not unregistered")
	}

	// Unregister non-existent (should be no-op)
	reg.Unregister("nonexistent") // Should not panic
}

func TestCollect(t *testing.T) {
	t.Run("single collector success", func(t *testing.T) {
		reg := NewRegistry(nil)
		collector := &mockCollector{
			name: "test",
			metrics: []Metric{
				{Name: "metric1", Value: 1.0},
				{Name: "metric2", Value: 2.0},
			},
		}
		reg.Register(collector)

		metrics, err := reg.Collect(context.Background())
		if err != nil {
			t.Fatalf("Collect failed: %v", err)
		}
		if len(metrics) != 2 {
			t.Errorf("Expected 2 metrics, got %d", len(metrics))
		}
	})

	t.Run("multiple collectors", func(t *testing.T) {
		reg := NewRegistry(nil)
		reg.Register(&mockCollector{
			name:    "collector1",
			metrics: []Metric{{Name: "m1", Value: 1.0}},
		})
		reg.Register(&mockCollector{
			name:    "collector2",
			metrics: []Metric{{Name: "m2", Value: 2.0}},
		})

		metrics, err := reg.Collect(context.Background())
		if err != nil {
			t.Fatalf("Collect failed: %v", err)
		}
		if len(metrics) != 2 {
			t.Errorf("Expected 2 metrics, got %d", len(metrics))
		}
	})

	t.Run("collector error (partial results)", func(t *testing.T) {
		reg := NewRegistry(nil)
		reg.Register(&mockCollector{
			name:    "good",
			metrics: []Metric{{Name: "m1", Value: 1.0}},
		})
		reg.Register(&mockCollector{
			name: "bad",
			err:  errors.New("test error"),
		})

		metrics, err := reg.Collect(context.Background())
		// Should return partial results with error
		if err == nil {
			t.Error("Expected error for partial failure, got nil")
		}
		if len(metrics) != 1 {
			t.Errorf("Expected 1 metric (partial results), got %d", len(metrics))
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		reg := NewRegistry(nil)
		reg.Register(&mockCollector{
			name:    "test",
			metrics: []Metric{{Name: "m1", Value: 1.0}},
		})

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		metrics, err := reg.Collect(ctx)
		if err == nil {
			t.Error("Expected error for cancelled context, got nil")
		}
		// May return partial results depending on timing
		if len(metrics) > 0 {
			t.Logf("Got %d partial results before cancellation", len(metrics))
		}
	})
}

func TestIsolation(t *testing.T) {
	t.Run("NoOpIsolator", func(t *testing.T) {
		isolator := &NoOpIsolator{}
		reg := NewRegistry(isolator)
		reg.Register(&mockCollector{
			name: "test",
			metrics: []Metric{
				{Name: "m1", Value: 1.0, Labels: []Label{{Key: "agent", Value: "test"}}},
			},
		})

		metrics, err := reg.Collect(context.Background())
		if err != nil {
			t.Fatalf("Collect failed: %v", err)
		}
		if len(metrics) != 1 {
			t.Fatalf("Expected 1 metric, got %d", len(metrics))
		}

		// NoOp should not modify labels
		if len(metrics[0].Labels) != 1 {
			t.Errorf("Expected 1 label, got %d", len(metrics[0].Labels))
		}
	})

	t.Run("LabelInjectionIsolator", func(t *testing.T) {
		isolator := &LabelInjectionIsolator{
			ExtraLabels: []Label{
				{Key: "org_id", Value: "org-123"},
				{Key: "tenant_id", Value: "tenant-456"},
			},
		}
		reg := NewRegistry(isolator)
		reg.Register(&mockCollector{
			name: "test",
			metrics: []Metric{
				{Name: "m1", Value: 1.0, Labels: []Label{{Key: "agent", Value: "test"}}},
			},
		})

		metrics, err := reg.Collect(context.Background())
		if err != nil {
			t.Fatalf("Collect failed: %v", err)
		}
		if len(metrics) != 1 {
			t.Fatalf("Expected 1 metric, got %d", len(metrics))
		}

		// Should have original label + 2 extra labels
		if len(metrics[0].Labels) != 3 {
			t.Errorf("Expected 3 labels (agent + org_id + tenant_id), got %d", len(metrics[0].Labels))
		}

		// Verify extra labels were added
		hasOrgID := false
		hasTenantID := false
		for _, label := range metrics[0].Labels {
			if label.Key == "org_id" && label.Value == "org-123" {
				hasOrgID = true
			}
			if label.Key == "tenant_id" && label.Value == "tenant-456" {
				hasTenantID = true
			}
		}
		if !hasOrgID || !hasTenantID {
			t.Error("Extra labels not properly injected")
		}
	})
}

func TestMetricType(t *testing.T) {
	tests := []struct {
		metricType MetricType
		expected   string
	}{
		{MetricTypeGauge, "gauge"},
		{MetricTypeCounter, "counter"},
		{MetricTypeHistogram, "histogram"},
		{MetricType(999), "untyped"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.metricType.String(); got != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, got)
			}
		})
	}
}

func TestLLMLatencyBuckets(t *testing.T) {
	// Verify bucket count
	if len(LLMLatencyBuckets) != 11 {
		t.Errorf("Expected 11 buckets, got %d", len(LLMLatencyBuckets))
	}

	// Verify buckets are sorted and cover expected range
	if LLMLatencyBuckets[0] != 0.1 {
		t.Errorf("Expected first bucket to be 0.1, got %f", LLMLatencyBuckets[0])
	}
	if LLMLatencyBuckets[len(LLMLatencyBuckets)-1] != 600.0 {
		t.Errorf("Expected last bucket to be 600.0, got %f", LLMLatencyBuckets[len(LLMLatencyBuckets)-1])
	}

	// Verify monotonically increasing
	for i := 1; i < len(LLMLatencyBuckets); i++ {
		if LLMLatencyBuckets[i] <= LLMLatencyBuckets[i-1] {
			t.Errorf("Buckets not monotonically increasing at index %d", i)
		}
	}
}

func BenchmarkCollect(b *testing.B) {
	reg := NewRegistry(nil)
	for i := 0; i < 10; i++ {
		reg.Register(&mockCollector{
			name: "collector-" + string(rune(i)),
			metrics: []Metric{
				{Name: "metric1", Value: 1.0, Labels: []Label{{Key: "agent", Value: "test"}}},
				{Name: "metric2", Value: 2.0, Labels: []Label{{Key: "agent", Value: "test"}}},
			},
		})
	}

	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := reg.Collect(ctx)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkIsolation(b *testing.B) {
	isolator := &LabelInjectionIsolator{
		ExtraLabels: []Label{
			{Key: "org_id", Value: "org-123"},
			{Key: "tenant_id", Value: "tenant-456"},
		},
	}

	metric := Metric{
		Name:      "test_metric",
		Type:      MetricTypeGauge,
		Value:     1.0,
		Labels:    []Label{{Key: "agent", Value: "test"}},
		Timestamp: time.Now().UnixMilli(),
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = isolator.Isolate(metric)
	}
}
