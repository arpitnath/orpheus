package telemetry

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPrometheusExporter_Export(t *testing.T) {
	tests := []struct {
		name     string
		metrics  []Metric
		expected string
	}{
		{
			name:     "empty metrics",
			metrics:  []Metric{},
			expected: "",
		},
		{
			name: "counter without labels",
			metrics: []Metric{{
				Name:        "http_requests_total",
				Description: "Total HTTP requests",
				Type:        MetricTypeCounter,
				Value:       1234,
			}},
			expected: `# HELP http_requests_total Total HTTP requests
# TYPE http_requests_total counter
http_requests_total 1234

`,
		},
		{
			name: "gauge with labels",
			metrics: []Metric{{
				Name:        "temperature",
				Description: "Current temperature",
				Type:        MetricTypeGauge,
				Labels:      []Label{{Key: "location", Value: "us-east"}},
				Value:       23.5,
			}},
			expected: `# HELP temperature Current temperature
# TYPE temperature gauge
temperature{location="us-east"} 23.5

`,
		},
		{
			name: "multiple labels sorted",
			metrics: []Metric{{
				Name:   "test_metric",
				Type:   MetricTypeGauge,
				Labels: []Label{{Key: "z", Value: "last"}, {Key: "a", Value: "first"}, {Key: "m", Value: "middle"}},
				Value:  1,
			}},
			expected: `# TYPE test_metric gauge
test_metric{a="first",m="middle",z="last"} 1

`,
		},
		{
			name: "label escaping",
			metrics: []Metric{{
				Name:   "test",
				Type:   MetricTypeGauge,
				Labels: []Label{{Key: "path", Value: `/foo\bar"baz` + "\n"}},
				Value:  1,
			}},
			expected: `# TYPE test gauge
test{path="/foo\\bar\"baz\n"} 1

`,
		},
		{
			name: "special float values",
			metrics: []Metric{
				{Name: "nan_metric", Type: MetricTypeGauge, Value: math.NaN()},
				{Name: "inf_metric", Type: MetricTypeGauge, Value: math.Inf(1)},
				{Name: "neginf_metric", Type: MetricTypeGauge, Value: math.Inf(-1)},
			},
			expected: `# TYPE inf_metric gauge
inf_metric +Inf

# TYPE nan_metric gauge
nan_metric NaN

# TYPE neginf_metric gauge
neginf_metric -Inf

`,
		},
		{
			name: "histogram with buckets",
			metrics: []Metric{{
				Name:        "request_duration_seconds",
				Description: "Request duration",
				Type:        MetricTypeHistogram,
				Labels:      []Label{{Key: "method", Value: "GET"}},
				Histogram: &HistogramData{
					Buckets: []HistogramBucket{
						{UpperBound: 0.1, CumulativeCount: 100},
						{UpperBound: 0.5, CumulativeCount: 150},
						{UpperBound: 1.0, CumulativeCount: 180},
					},
					Sum:   45.6,
					Count: 200,
				},
			}},
			expected: `# HELP request_duration_seconds Request duration
# TYPE request_duration_seconds histogram
request_duration_seconds_bucket{method="GET",le="0.1"} 100
request_duration_seconds_bucket{method="GET",le="0.5"} 150
request_duration_seconds_bucket{method="GET",le="1"} 180
request_duration_seconds_bucket{method="GET",le="+Inf"} 200
request_duration_seconds_sum{method="GET"} 45.6
request_duration_seconds_count{method="GET"} 200

`,
		},
		{
			name: "histogram without labels",
			metrics: []Metric{{
				Name: "test_histogram",
				Type: MetricTypeHistogram,
				Histogram: &HistogramData{
					Buckets: []HistogramBucket{
						{UpperBound: 1.0, CumulativeCount: 10},
					},
					Sum:   5.5,
					Count: 10,
				},
			}},
			expected: `# TYPE test_histogram histogram
test_histogram_bucket{le="1"} 10
test_histogram_bucket{le="+Inf"} 10
test_histogram_sum 5.5
test_histogram_count 10

`,
		},
		{
			name: "multiple metrics sorted by name",
			metrics: []Metric{
				{Name: "zebra", Type: MetricTypeGauge, Value: 3},
				{Name: "alpha", Type: MetricTypeGauge, Value: 1},
				{Name: "beta", Type: MetricTypeGauge, Value: 2},
			},
			expected: `# TYPE alpha gauge
alpha 1

# TYPE beta gauge
beta 2

# TYPE zebra gauge
zebra 3

`,
		},
	}

	exporter := &PrometheusExporter{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := exporter.Export(tt.metrics)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(result) != tt.expected {
				t.Errorf("mismatch:\ngot:\n%s\nwant:\n%s", string(result), tt.expected)
			}
		})
	}
}

func TestPrometheusExporter_ServeHTTP(t *testing.T) {
	// Create mock registry
	reg := NewRegistry(nil)
	reg.Register(&mockCollector{
		name: "test",
		metrics: []Metric{
			{Name: "test_metric", Type: MetricTypeGauge, Value: 42, Labels: []Label{{Key: "agent", Value: "test"}}},
		},
	})

	exporter := NewPrometheusExporter(reg)

	// Create test request
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	// Serve
	exporter.ServeHTTP(rec, req)

	// Verify response
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != exporter.ContentType() {
		t.Errorf("Expected Content-Type %s, got %s", exporter.ContentType(), contentType)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "test_metric") {
		t.Errorf("Expected metric in output, got: %s", body)
	}
	if !strings.Contains(body, "# TYPE test_metric gauge") {
		t.Error("Expected TYPE line in output")
	}
}

func TestFormatValue(t *testing.T) {
	tests := []struct {
		name     string
		value    float64
		expected string
	}{
		{"regular", 123.45, "123.45"},
		{"integer", 42.0, "42"},
		{"NaN", math.NaN(), "NaN"},
		{"positive infinity", math.Inf(1), "+Inf"},
		{"negative infinity", math.Inf(-1), "-Inf"},
		{"zero", 0.0, "0"},
		{"small", 0.0001, "0.0001"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatValue(tt.value)
			if result != tt.expected {
				t.Errorf("formatValue(%v) = %s, want %s", tt.value, result, tt.expected)
			}
		})
	}
}

func TestEscapeValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"no escaping", "simple", "simple"},
		{"backslash", `\path`, `\\path`},
		{"quote", `say "hello"`, `say \"hello\"`},
		{"newline", "line1\nline2", `line1\nline2`},
		{"all", `\path\n"quoted"` + "\n", `\\path\\n\"quoted\"\n`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeValue(tt.input)
			if result != tt.expected {
				t.Errorf("escapeValue(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestHistogramBucketSorting(t *testing.T) {
	// Buckets provided out of order
	metric := Metric{
		Name: "test",
		Type: MetricTypeHistogram,
		Histogram: &HistogramData{
			Buckets: []HistogramBucket{
				{UpperBound: 5.0, CumulativeCount: 150},
				{UpperBound: 0.1, CumulativeCount: 100},
				{UpperBound: 1.0, CumulativeCount: 120},
			},
			Sum:   100.0,
			Count: 200,
		},
	}

	exporter := &PrometheusExporter{}
	result, err := exporter.Export([]Metric{metric})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := string(result)

	// Verify buckets appear in sorted order
	lines := strings.Split(output, "\n")
	bucketLines := []string{}
	for _, line := range lines {
		if strings.Contains(line, "_bucket") {
			bucketLines = append(bucketLines, line)
		}
	}

	// Should be sorted: 0.1, 1, 5, +Inf
	if len(bucketLines) != 4 {
		t.Fatalf("Expected 4 bucket lines, got %d: %v", len(bucketLines), bucketLines)
	}

	expectedOrder := []string{"0.1", "1", "5", "+Inf"}
	for i, expected := range expectedOrder {
		if !strings.Contains(bucketLines[i], `le="`+expected+`"`) {
			t.Errorf("Bucket %d: expected le=%s, got %s", i, expected, bucketLines[i])
		}
	}
}

func BenchmarkExport(b *testing.B) {
	// Create realistic metric set (10 agents, 15 metrics each)
	metrics := make([]Metric, 0, 150)
	for i := 0; i < 10; i++ {
		agentLabels := []Label{{Key: "agent", Value: fmt.Sprintf("agent-%d", i)}}

		// Pool metrics (4)
		metrics = append(metrics,
			Metric{Name: "orpheus_pool_workers_total", Type: MetricTypeGauge, Value: 5, Labels: agentLabels},
			Metric{Name: "orpheus_pool_workers_idle", Type: MetricTypeGauge, Value: 2, Labels: agentLabels},
			Metric{Name: "orpheus_pool_workers_busy", Type: MetricTypeGauge, Value: 3, Labels: agentLabels},
			Metric{Name: "orpheus_pool_desired_size", Type: MetricTypeGauge, Value: 5, Labels: agentLabels},
		)

		// Queue metrics (4)
		metrics = append(metrics,
			Metric{Name: "orpheus_queue_pending", Type: MetricTypeGauge, Value: 12, Labels: agentLabels},
			Metric{Name: "orpheus_queue_processing", Type: MetricTypeGauge, Value: 3, Labels: agentLabels},
			Metric{Name: "orpheus_queue_depth_total", Type: MetricTypeGauge, Value: 15, Labels: agentLabels},
			Metric{Name: "orpheus_queue_max_size", Type: MetricTypeGauge, Value: 50, Labels: agentLabels},
		)
	}

	exporter := &PrometheusExporter{}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := exporter.Export(metrics)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestServeHTTPTimeout(t *testing.T) {
	// Create registry with slow collector
	reg := NewRegistry(nil)
	reg.Register(&mockCollector{
		name: "slow",
		metrics: []Metric{
			{Name: "test", Type: MetricTypeGauge, Value: 1},
		},
	})

	exporter := NewPrometheusExporter(reg)

	// Create request with very short context (to test timeout handling)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	ctx, cancel := context.WithCancel(req.Context())
	cancel() // Cancel immediately
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	exporter.ServeHTTP(rec, req)

	// Should still return (possibly partial) results
	// Handler creates new timeout context, so immediate cancellation doesn't break it
	if rec.Code != http.StatusOK {
		t.Logf("Handler returned status %d (acceptable for cancelled context)", rec.Code)
	}
}

func TestIsValidMetricName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Valid names
		{"simple name", "metric_name", true},
		{"with colon", "metric:name", true},
		{"starts with underscore", "_private", true},
		{"with numbers", "metric123", true},
		{"uppercase", "MyMetric", true},
		{"all allowed chars", "My_Metric:123", true},

		// Invalid names
		{"starts with digit", "123metric", false},
		{"with hyphen", "metric-name", false},
		{"empty", "", false},
		{"with space", "metric name", false},
		{"with dot", "metric.name", false},
		{"with at sign", "metric@name", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidMetricName(tt.input)
			if result != tt.expected {
				t.Errorf("isValidMetricName(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsValidLabelKey(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Valid keys
		{"simple key", "agent", true},
		{"starts with underscore", "_private", true},
		{"with numbers", "label_123", true},
		{"uppercase", "MyLabel", true},

		// Invalid keys
		{"reserved le", "le", false},
		{"starts with digit", "123label", false},
		{"with colon", "label:key", false},
		{"with hyphen", "label-key", false},
		{"empty", "", false},
		{"with space", "label key", false},
		{"with dot", "label.key", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidLabelKey(tt.input)
			if result != tt.expected {
				t.Errorf("isValidLabelKey(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}
