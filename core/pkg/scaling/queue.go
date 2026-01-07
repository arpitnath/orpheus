package scaling

import (
	"context"
	"errors"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// Queue errors.
var (
	ErrQueueFull   = errors.New("queue is full")
	ErrQueueClosed = errors.New("queue is closed")
)

// Request represents a queued agent execution request.
type Request struct {
	ID         string
	Input      []byte
	Context    context.Context
	ResponseCh chan *Response
	StreamCh   chan *StreamEvent // Optional: for SSE streaming (nil for non-streaming requests)
	QueuedAt   time.Time
	StartedAt  *time.Time
	SessionID  string // Optional: session ID for worker affinity (from X-Session-ID header)
}

// Response represents the result of an agent execution.
type Response struct {
	Result   *Result
	Error    error
	Duration time.Duration
}

// StreamEvent represents a real-time event during agent execution (for SSE).
type StreamEvent struct {
	Type      string                 `json:"type"`      // "init", "chunk", "progress", "error", "completed"
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
}

// RequestQueue implements QueueMetrics and manages request flow.
// It connects incoming HTTP requests to workers and provides
// the metrics the autoscaler needs (pending + processing counts).
type RequestQueue struct {
	agentID string
	maxSize int

	pending    chan *Request // Buffered channel for pending requests
	processing sync.Map      // Map of request ID -> *Request (currently processing)

	pendingCount    atomic.Int32 // Fast access for metrics
	processingCount atomic.Int32

	mu     sync.RWMutex
	closed atomic.Bool
}

// NewRequestQueue creates a new request queue for the given agent.
// maxSize determines the maximum number of pending requests (tier-based).
// If maxSize <= 0, the queue is unbounded (not recommended for production).
func NewRequestQueue(agentID string, maxSize int) *RequestQueue {
	// Use maxSize for channel buffer, minimum 1
	bufferSize := maxSize
	if bufferSize <= 0 {
		bufferSize = 100 // Default buffer if unbounded
	}

	q := &RequestQueue{
		agentID: agentID,
		maxSize: maxSize,
		pending: make(chan *Request, bufferSize),
	}

	log.Printf("[queue] Created queue for %s (maxSize=%d)", agentID, maxSize)
	return q
}

// AgentID returns the identifier of the agent this queue serves.
func (q *RequestQueue) AgentID() string {
	return q.agentID
}

// Enqueue adds a request to the queue.
// Returns ErrQueueFull if at capacity, ErrQueueClosed if closed.
func (q *RequestQueue) Enqueue(ctx context.Context, req *Request) error {
	if q.closed.Load() {
		return ErrQueueClosed
	}

	// Check queue capacity
	if q.maxSize > 0 && int(q.pendingCount.Load()) >= q.maxSize {
		return ErrQueueFull
	}

	// Set queue timestamp
	req.QueuedAt = time.Now()

	// Try to enqueue
	select {
	case q.pending <- req:
		q.pendingCount.Add(1)
		log.Printf("[queue] %s: enqueued request %s (pending=%d)",
			q.agentID, req.ID, q.pendingCount.Load())
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		// Channel full (race condition with capacity check)
		return ErrQueueFull
	}
}

// Dequeue returns the next request, blocking until available.
// Moves the request from pending to processing state.
func (q *RequestQueue) Dequeue(ctx context.Context) (*Request, error) {
	select {
	case req := <-q.pending:
		q.pendingCount.Add(-1)

		// Move to processing
		now := time.Now()
		req.StartedAt = &now
		q.processing.Store(req.ID, req)
		q.processingCount.Add(1)

		log.Printf("[queue] %s: dequeued request %s (pending=%d, processing=%d)",
			q.agentID, req.ID, q.pendingCount.Load(), q.processingCount.Load())
		return req, nil

	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Complete marks a request as done and removes it from processing.
// Should be called after the worker finishes executing the request.
func (q *RequestQueue) Complete(requestID string) {
	if _, loaded := q.processing.LoadAndDelete(requestID); loaded {
		q.processingCount.Add(-1)
		log.Printf("[queue] %s: completed request %s (processing=%d)",
			q.agentID, requestID, q.processingCount.Load())
	}
}

// PendingTasks returns the number of tasks waiting in the queue.
// Implements QueueMetrics interface.
func (q *RequestQueue) PendingTasks() int {
	return int(q.pendingCount.Load())
}

// ProcessingTasks returns the number of tasks currently being processed.
// Implements QueueMetrics interface.
func (q *RequestQueue) ProcessingTasks() int {
	return int(q.processingCount.Load())
}

// QueueLength returns the total number of tasks (pending + processing).
// Implements QueueMetrics interface.
func (q *RequestQueue) QueueLength() int {
	return q.PendingTasks() + q.ProcessingTasks()
}

// IsFull returns true if the queue is at capacity.
func (q *RequestQueue) IsFull() bool {
	if q.maxSize <= 0 {
		return false // Unbounded queue
	}
	return int(q.pendingCount.Load()) >= q.maxSize
}

// IsClosed returns true if the queue has been closed.
func (q *RequestQueue) IsClosed() bool {
	return q.closed.Load()
}

// Close stops accepting new requests.
// Existing pending requests can still be dequeued.
// Processing requests can still be completed.
func (q *RequestQueue) Close() error {
	if q.closed.Swap(true) {
		return nil // Already closed
	}

	log.Printf("[queue] %s: closing (pending=%d, processing=%d)",
		q.agentID, q.pendingCount.Load(), q.processingCount.Load())

	// Note: We don't close the channel here to allow draining.
	// The channel will be garbage collected when the queue is no longer referenced.
	return nil
}

// Drain removes and returns all pending requests without processing them.
// Useful for graceful shutdown - allows returning errors to waiting clients.
func (q *RequestQueue) Drain() []*Request {
	var drained []*Request

	for {
		select {
		case req := <-q.pending:
			q.pendingCount.Add(-1)
			drained = append(drained, req)
		default:
			// Channel empty
			log.Printf("[queue] %s: drained %d requests", q.agentID, len(drained))
			return drained
		}
	}
}

// Stats returns a snapshot of the queue's current state.
type QueueStats struct {
	AgentID         string    `json:"agent_id"`
	PendingCount    int       `json:"pending_count"`
	ProcessingCount int       `json:"processing_count"`
	MaxSize         int       `json:"max_size"`
	IsClosed        bool      `json:"is_closed"`
	Timestamp       time.Time `json:"timestamp"`
}

// GetStats returns a snapshot of the queue's current statistics.
func (q *RequestQueue) GetStats() QueueStats {
	return QueueStats{
		AgentID:         q.agentID,
		PendingCount:    q.PendingTasks(),
		ProcessingCount: q.ProcessingTasks(),
		MaxSize:         q.maxSize,
		IsClosed:        q.IsClosed(),
		Timestamp:       time.Now(),
	}
}
