package telemetry

// MetricType indicates the semantic type of a metric.
type MetricType int

const (
	// MetricTypeGauge represents a value that can go up or down.
	// Examples: queue depth, worker count, memory usage.
	MetricTypeGauge MetricType = iota

	// MetricTypeCounter represents a monotonically increasing value.
	// Examples: total requests, total errors.
	MetricTypeCounter

	// MetricTypeHistogram represents a distribution of values.
	// Examples: request duration, response size.
	MetricTypeHistogram
)

// String returns the string representation of MetricType.
func (t MetricType) String() string {
	switch t {
	case MetricTypeGauge:
		return "gauge"
	case MetricTypeCounter:
		return "counter"
	case MetricTypeHistogram:
		return "histogram"
	default:
		return "untyped"
	}
}

// Label is a key-value pair for metric dimensions.
// Labels MUST use bounded cardinality (no request_id, user_id, etc).
type Label struct {
	Key   string // Label name (e.g., "agent", "status", "engine")
	Value string // Label value (e.g., "calculator", "completed", "ollama")
}

// Metric represents a single metric data point.
type Metric struct {
	Name        string     // Metric name (e.g., "orpheus_pool_workers_total")
	Type        MetricType // Semantic type (gauge, counter, histogram)
	Value       float64    // Current value (for gauge/counter)
	Labels      []Label    // Dimensional labels (bounded cardinality only)
	Description string     // Human-readable description (for HELP lines)
	Timestamp   int64      // Unix milliseconds (0 = omit timestamp)

	// Histogram-specific data (nil for non-histogram metrics)
	Histogram *HistogramData
}

// HistogramData contains bucket distribution data for histogram metrics.
type HistogramData struct {
	Buckets []HistogramBucket // Sorted buckets with cumulative counts
	Sum     float64           // Sum of all observed values
	Count   uint64            // Total number of observations
}

// HistogramBucket represents a single bucket in a histogram distribution.
type HistogramBucket struct {
	UpperBound      float64 // Inclusive upper bound (le="X")
	CumulativeCount uint64  // Cumulative count of observations <= UpperBound
}

// LLMLatencyBuckets defines histogram buckets optimized for LLM workloads.
// Standard Prometheus buckets (0.005s-10s) miss most LLM inference times.
// These buckets cover: 100ms (fast) to 600s (10min max timeout).
var LLMLatencyBuckets = []float64{
	0.1,   // 100ms - fast local inference
	0.5,   // 500ms - typical small model
	1.0,   // 1s - standard LLM call
	2.5,   // 2.5s - complex prompts
	5.0,   // 5s - multi-step reasoning
	10.0,  // 10s - RAG with retrieval
	30.0,  // 30s - long-form generation
	60.0,  // 1min - complex workflows
	120.0, // 2min - multi-agent collaboration
	300.0, // 5min - extended workflows
	600.0, // 10min - absolute maximum timeout
}

// QueueDepthBuckets defines histogram buckets for queue depth distribution.
var QueueDepthBuckets = []float64{
	1, 5, 10, 25, 50, 100, 250, 500, 1000,
}
