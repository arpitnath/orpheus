package runtime

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// StreamEvent represents a Server-Sent Event for real-time agent execution updates.
type StreamEvent struct {
	Type      string                 `json:"type"`      // Event type: init, chunk, metrics, progress, completed, error
	Timestamp time.Time              `json:"timestamp"` // When the event occurred
	Data      map[string]interface{} `json:"data"`      // Event-specific data
}

// StreamWriter interface for sending real-time events during agent execution.
type StreamWriter interface {
	WriteEvent(event *StreamEvent) error
	Close() error
}

// SSEWriter implements StreamWriter for Server-Sent Events protocol.
// Thread-safe for concurrent event writes.
type SSEWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
	mu      sync.Mutex
	closed  bool
}

// NewSSEWriter creates a new SSE writer from an http.ResponseWriter.
// Returns nil if the writer doesn't support flushing (required for SSE).
func NewSSEWriter(w http.ResponseWriter) *SSEWriter {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil
	}

	return &SSEWriter{
		w:       w,
		flusher: flusher,
	}
}

// WriteEvent writes a single SSE event to the client.
// Format: event: <type>\ndata: <json>\n\n
// Thread-safe.
func (s *SSEWriter) WriteEvent(event *StreamEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("writer closed")
	}

	// Marshal event data to JSON
	dataJSON, err := json.Marshal(event.Data)
	if err != nil {
		return fmt.Errorf("marshal event data: %w", err)
	}

	// Write SSE format:
	// event: <type>
	// data: <json>
	// <blank line>
	_, err = fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", event.Type, string(dataJSON))
	if err != nil {
		return fmt.Errorf("write event: %w", err)
	}

	// Flush immediately (required for SSE real-time delivery)
	s.flusher.Flush()

	return nil
}

// Close marks the writer as closed.
// No cleanup needed for SSE (HTTP layer handles connection).
func (s *SSEWriter) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closed = true
	return nil
}

// NewChunkEvent creates a chunk event for stdout/stderr output.
func NewChunkEvent(stream string, content string) *StreamEvent {
	return &StreamEvent{
		Type:      "chunk",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"stream":  stream, // "stdout" or "stderr"
			"content": content,
		},
	}
}

// NewProgressEvent creates a progress event with execution metrics.
func NewProgressEvent(elapsedMs int64, memoryMb int) *StreamEvent {
	return &StreamEvent{
		Type:      "progress",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"elapsed_ms": elapsedMs,
			"memory_mb":  memoryMb,
		},
	}
}

// NewCompletedEvent creates a completion event with final result.
func NewCompletedEvent(status string, durationMs int64, output map[string]interface{}) *StreamEvent {
	return &StreamEvent{
		Type:      "completed",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"status":      status,
			"duration_ms": durationMs,
			"output":      output,
		},
	}
}

// NewErrorEvent creates an error event.
func NewErrorEvent(errorMsg string, exitCode int) *StreamEvent {
	return &StreamEvent{
		Type:      "error",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"error":     errorMsg,
			"exit_code": exitCode,
		},
	}
}
