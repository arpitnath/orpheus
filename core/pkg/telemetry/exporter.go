package telemetry

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// bufferPool reuses byte buffers for export to reduce allocations.
var bufferPool = sync.Pool{
	New: func() interface{} {
		return bytes.NewBuffer(make([]byte, 0, 4096))
	},
}

// PrometheusExporter exports metrics in Prometheus text format.
// Implements http.Handler for the /metrics endpoint.
type PrometheusExporter struct {
	registry MetricRegistry
}

// NewPrometheusExporter creates an exporter that writes Prometheus text format.
func NewPrometheusExporter(registry MetricRegistry) *PrometheusExporter {
	return &PrometheusExporter{
		registry: registry,
	}
}

// ContentType returns the MIME type for Prometheus text format.
func (e *PrometheusExporter) ContentType() string {
	return "text/plain; version=0.0.4; charset=utf-8"
}

// Export formats metrics in Prometheus text format.
// Returns empty bytes (not error) if metrics slice is empty.
func (e *PrometheusExporter) Export(metrics []Metric) ([]byte, error) {
	if len(metrics) == 0 {
		return []byte{}, nil
	}

	// Get buffer from pool
	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufferPool.Put(buf)

	// Group metrics by name
	grouped := make(map[string][]Metric)
	for _, m := range metrics {
		grouped[m.Name] = append(grouped[m.Name], m)
	}

	// Sort metric names for deterministic output
	names := make([]string, 0, len(grouped))
	for name := range grouped {
		names = append(names, name)
	}
	sort.Strings(names)

	// Write each metric group
	for _, name := range names {
		// Validate metric name to prevent injection
		if !isValidMetricName(name) {
			log.Printf("[telemetry] Skipping invalid metric name: %q", name)
			continue
		}

		metricGroup := grouped[name]
		first := metricGroup[0]

		// Write HELP and TYPE (once per metric name)
		if first.Description != "" {
			fmt.Fprintf(buf, "# HELP %s %s\n", name, escapeHelp(first.Description))
		}
		fmt.Fprintf(buf, "# TYPE %s %s\n", name, first.Type.String())

		// Write samples
		for _, m := range metricGroup {
			if m.Type == MetricTypeHistogram {
				writeHistogram(buf, m)
			} else {
				writeSample(buf, m)
			}
		}
		buf.WriteByte('\n')
	}

	// Copy result (buffer returns to pool)
	result := make([]byte, buf.Len())
	copy(result, buf.Bytes())
	return result, nil
}

// ServeHTTP implements http.Handler for the /metrics endpoint.
func (e *PrometheusExporter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Create context with timeout to prevent hung scrapes
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Collect metrics from registry
	metrics, err := e.registry.Collect(ctx)
	if err != nil {
		// Partial results are acceptable - log but continue
		log.Printf("[telemetry] Metric collection error (returning partial results): %v", err)
	}

	// Export to Prometheus format
	output, err := e.Export(metrics)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to export metrics: %v", err), http.StatusInternalServerError)
		return
	}

	// Write response
	w.Header().Set("Content-Type", e.ContentType())
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(output); err != nil {
		// Response already committed, can't send error to client
		// Log for debugging network issues
		log.Printf("[telemetry] Failed to write metrics response: %v", err)
	}
}

// writeSample writes a gauge or counter metric line.
func writeSample(buf *bytes.Buffer, m Metric) {
	labels := formatLabels(m.Labels)
	value := formatValue(m.Value)

	// Include timestamp if provided (milliseconds)
	if m.Timestamp > 0 {
		fmt.Fprintf(buf, "%s%s %s %d\n", m.Name, labels, value, m.Timestamp)
	} else {
		fmt.Fprintf(buf, "%s%s %s\n", m.Name, labels, value)
	}
}

// writeHistogram writes a histogram metric with buckets.
func writeHistogram(buf *bytes.Buffer, m Metric) {
	if m.Histogram == nil {
		return
	}

	// Sort buckets by upper bound (required by spec)
	buckets := make([]HistogramBucket, len(m.Histogram.Buckets))
	copy(buckets, m.Histogram.Buckets)
	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i].UpperBound < buckets[j].UpperBound
	})

	// Format base labels (without closing brace for histogram)
	baseLabels := formatLabelsForHistogram(m.Labels)
	hasInf := false

	// Write buckets
	for _, bucket := range buckets {
		le := formatLeValue(bucket.UpperBound)
		if math.IsInf(bucket.UpperBound, 1) {
			hasInf = true
		}

		if baseLabels == "" {
			fmt.Fprintf(buf, "%s_bucket{le=\"%s\"} %d\n",
				m.Name, le, bucket.CumulativeCount)
		} else {
			fmt.Fprintf(buf, "%s_bucket{%s,le=\"%s\"} %d\n",
				m.Name, baseLabels, le, bucket.CumulativeCount)
		}
	}

	// Ensure +Inf bucket exists (required by spec)
	if !hasInf {
		if baseLabels == "" {
			fmt.Fprintf(buf, "%s_bucket{le=\"+Inf\"} %d\n",
				m.Name, m.Histogram.Count)
		} else {
			fmt.Fprintf(buf, "%s_bucket{%s,le=\"+Inf\"} %d\n",
				m.Name, baseLabels, m.Histogram.Count)
		}
	}

	// Write sum and count
	labels := formatLabels(m.Labels)
	fmt.Fprintf(buf, "%s_sum%s %s\n", m.Name, labels, formatValue(m.Histogram.Sum))
	fmt.Fprintf(buf, "%s_count%s %d\n", m.Name, labels, m.Histogram.Count)
}

// formatLabels formats labels with sorting for deterministic output.
// Returns "{key1="value1",key2="value2"}" or "" if no labels.
// Filters out invalid label keys to prevent injection attacks.
func formatLabels(labels []Label) string {
	if len(labels) == 0 {
		return ""
	}

	// Filter and sort labels by key for deterministic output
	valid := make([]Label, 0, len(labels))
	for _, label := range labels {
		if isValidLabelKey(label.Key) {
			valid = append(valid, label)
		} else {
			log.Printf("[telemetry] Skipping invalid label key: %q", label.Key)
		}
	}

	if len(valid) == 0 {
		return ""
	}

	sort.Slice(valid, func(i, j int) bool {
		return valid[i].Key < valid[j].Key
	})

	pairs := make([]string, len(valid))
	for i, label := range valid {
		pairs[i] = fmt.Sprintf(`%s="%s"`, label.Key, escapeValue(label.Value))
	}
	return "{" + strings.Join(pairs, ",") + "}"
}

// formatLabelsForHistogram formats labels without closing brace.
// Returns "key1="value1",key2="value2"" or "" if no labels.
// Used for histogram buckets where le label is appended.
// Filters out invalid label keys to prevent injection attacks.
func formatLabelsForHistogram(labels []Label) string {
	if len(labels) == 0 {
		return ""
	}

	// Filter and sort labels
	valid := make([]Label, 0, len(labels))
	for _, label := range labels {
		if isValidLabelKey(label.Key) {
			valid = append(valid, label)
		} else {
			log.Printf("[telemetry] Skipping invalid label key: %q", label.Key)
		}
	}

	if len(valid) == 0 {
		return ""
	}

	sort.Slice(valid, func(i, j int) bool {
		return valid[i].Key < valid[j].Key
	})

	pairs := make([]string, len(valid))
	for i, label := range valid {
		pairs[i] = fmt.Sprintf(`%s="%s"`, label.Key, escapeValue(label.Value))
	}
	return strings.Join(pairs, ",")
}

// formatValue formats a float64 value according to Prometheus spec.
// Handles special values: NaN, +Inf, -Inf.
func formatValue(v float64) string {
	switch {
	case math.IsNaN(v):
		return "NaN"
	case math.IsInf(v, 1):
		return "+Inf"
	case math.IsInf(v, -1):
		return "-Inf"
	default:
		return strconv.FormatFloat(v, 'g', -1, 64)
	}
}

// formatLeValue formats the le (less-than-or-equal) label value for histogram buckets.
func formatLeValue(v float64) string {
	if math.IsInf(v, 1) {
		return "+Inf"
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}

// escapeValue escapes special characters in label values per Prometheus spec.
// Escapes: backslash (\), double-quote ("), newline (\n).
func escapeValue(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}

// escapeHelp escapes special characters in HELP text.
// Escapes: backslash (\), newline (\n).
func escapeHelp(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}

// isValidMetricName validates a metric name against Prometheus spec.
// Valid names match: [a-zA-Z_:][a-zA-Z0-9_:]*
func isValidMetricName(name string) bool {
	if len(name) == 0 {
		return false
	}

	// First character: letter, underscore, or colon
	first := rune(name[0])
	if !isLetter(first) && first != '_' && first != ':' {
		return false
	}

	// Remaining characters: letter, digit, underscore, or colon
	for _, c := range name[1:] {
		if !isLetter(c) && !isDigit(c) && c != '_' && c != ':' {
			return false
		}
	}

	return true
}

// isValidLabelKey validates a label key against Prometheus spec.
// Valid keys match: [a-zA-Z_][a-zA-Z0-9_]*
// Also blocks reserved key "le" (used by histograms).
func isValidLabelKey(key string) bool {
	if len(key) == 0 {
		return false
	}

	// Reserved label for histograms
	if key == "le" {
		return false
	}

	// First character: letter or underscore (not colon, unlike metric names)
	first := rune(key[0])
	if !isLetter(first) && first != '_' {
		return false
	}

	// Remaining characters: letter, digit, or underscore
	for _, c := range key[1:] {
		if !isLetter(c) && !isDigit(c) && c != '_' {
			return false
		}
	}

	return true
}

// isLetter checks if rune is a letter (a-z, A-Z).
func isLetter(c rune) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// isDigit checks if rune is a digit (0-9).
func isDigit(c rune) bool {
	return c >= '0' && c <= '9'
}

// Ensure PrometheusExporter implements MetricExporter
var _ MetricExporter = (*PrometheusExporter)(nil)

