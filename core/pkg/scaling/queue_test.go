package scaling

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestNewRequestQueue(t *testing.T) {
	q := NewRequestQueue("test-agent", 10)

	if q == nil {
		t.Fatal("NewRequestQueue returned nil")
	}

	if q.PendingTasks() != 0 {
		t.Errorf("New queue should have 0 pending, got %d", q.PendingTasks())
	}

	if q.ProcessingTasks() != 0 {
		t.Errorf("New queue should have 0 processing, got %d", q.ProcessingTasks())
	}
}

func TestEnqueue_BelowCapacity(t *testing.T) {
	q := NewRequestQueue("test-agent", 5)

	req := &Request{
		ID:         "req-1",
		Input:      []byte(`{"test": "data"}`),
		ResponseCh: make(chan *Response, 1),
	}

	err := q.Enqueue(context.Background(), req)
	if err != nil {
		t.Fatalf("Enqueue below capacity should succeed, got error: %v", err)
	}

	// Pending should increase
	if q.PendingTasks() != 1 {
		t.Errorf("Expected 1 pending task, got %d", q.PendingTasks())
	}
}

func TestEnqueue_AtCapacity(t *testing.T) {
	q := NewRequestQueue("test-agent", 2) // Small capacity

	// Enqueue 2 requests (fill capacity)
	for i := 0; i < 2; i++ {
		req := &Request{
			ID:       fmt.Sprintf("req-%d", i),
			Input:    []byte("{}"),
			ResponseCh: make(chan *Response, 1),
		}
		err := q.Enqueue(context.Background(), req)
		if err != nil {
			t.Fatalf("Enqueue %d failed: %v", i, err)
		}
	}

	// Third enqueue should fail (queue full)
	req3 := &Request{
		ID:       "req-3",
		Input:    []byte("{}"),
		ResponseCh: make(chan *Response, 1),
	}

	err := q.Enqueue(context.Background(), req3)
	if err != ErrQueueFull {
		t.Errorf("Enqueue at capacity should return ErrQueueFull, got: %v", err)
	}

	// Pending should still be 2
	if q.PendingTasks() != 2 {
		t.Errorf("Expected 2 pending (capacity), got %d", q.PendingTasks())
	}
}

func TestDequeue_Blocking(t *testing.T) {
	q := NewRequestQueue("test-agent", 10)

	// Enqueue a request
	req := &Request{
		ID:       "req-1",
		Input:    []byte("{}"),
		ResponseCh: make(chan *Response, 1),
	}
	q.Enqueue(context.Background(), req)

	// Dequeue should return it immediately
	ctx := context.Background()
	dequeued, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}

	if dequeued.ID != "req-1" {
		t.Errorf("Expected req-1, got %s", dequeued.ID)
	}

	// Pending should be 0, processing should be 1
	if q.PendingTasks() != 0 {
		t.Errorf("After dequeue, pending should be 0, got %d", q.PendingTasks())
	}

	if q.ProcessingTasks() != 1 {
		t.Errorf("After dequeue, processing should be 1, got %d", q.ProcessingTasks())
	}
}

func TestDequeue_ContextCancel(t *testing.T) {
	q := NewRequestQueue("test-agent", 10)

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Dequeue from empty queue (will block)
	_, err := q.Dequeue(ctx)

	// Should return context error
	if err != context.DeadlineExceeded {
		t.Errorf("Expected DeadlineExceeded, got: %v", err)
	}
}

func TestComplete(t *testing.T) {
	q := NewRequestQueue("test-agent", 10)

	req := &Request{
		ID:       "req-1",
		Input:    []byte("{}"),
		ResponseCh: make(chan *Response, 1),
	}

	// Enqueue and dequeue
	q.Enqueue(context.Background(), req)
	q.Dequeue(context.Background())

	// Processing should be 1
	if q.ProcessingTasks() != 1 {
		t.Errorf("Before complete, processing should be 1, got %d", q.ProcessingTasks())
	}

	// Complete
	q.Complete("req-1")

	// Processing should be 0
	if q.ProcessingTasks() != 0 {
		t.Errorf("After complete, processing should be 0, got %d", q.ProcessingTasks())
	}
}

func TestPendingTasks_Accuracy(t *testing.T) {
	q := NewRequestQueue("test-agent", 10)

	// Enqueue 5 requests
	for i := 0; i < 5; i++ {
		req := &Request{
			ID:       fmt.Sprintf("req-%d", i),
			Input:    []byte("{}"),
			ResponseCh: make(chan *Response, 1),
		}
		q.Enqueue(context.Background(), req)
	}

	// Should show 5 pending
	if q.PendingTasks() != 5 {
		t.Errorf("Expected 5 pending, got %d", q.PendingTasks())
	}

	// Dequeue one
	q.Dequeue(context.Background())

	// Should show 4 pending, 1 processing
	if q.PendingTasks() != 4 {
		t.Errorf("After dequeue, expected 4 pending, got %d", q.PendingTasks())
	}
}

func TestProcessingTasks_Accuracy(t *testing.T) {
	q := NewRequestQueue("test-agent", 10)

	// Enqueue and dequeue 3 requests (move to processing)
	for i := 0; i < 3; i++ {
		req := &Request{
			ID:       fmt.Sprintf("req-%d", i),
			Input:    []byte("{}"),
			ResponseCh: make(chan *Response, 1),
		}
		q.Enqueue(context.Background(), req)
		q.Dequeue(context.Background())
	}

	// Should show 3 processing
	if q.ProcessingTasks() != 3 {
		t.Errorf("Expected 3 processing, got %d", q.ProcessingTasks())
	}

	// Complete one
	q.Complete("req-1")

	// Should show 2 processing
	if q.ProcessingTasks() != 2 {
		t.Errorf("After complete, expected 2 processing, got %d", q.ProcessingTasks())
	}
}

func TestClose(t *testing.T) {
	q := NewRequestQueue("test-agent", 10)

	// Enqueue before close
	req1 := &Request{
		ID:       "req-1",
		Input:    []byte("{}"),
		ResponseCh: make(chan *Response, 1),
	}
	err := q.Enqueue(context.Background(), req1)
	if err != nil {
		t.Fatalf("Enqueue before close failed: %v", err)
	}

	// Close queue
	err = q.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Enqueue after close should fail
	req2 := &Request{
		ID:       "req-2",
		Input:    []byte("{}"),
		ResponseCh: make(chan *Response, 1),
	}
	err = q.Enqueue(context.Background(), req2)
	if err != ErrQueueClosed {
		t.Errorf("Enqueue after close should return ErrQueueClosed, got: %v", err)
	}

	// Dequeue should still work (drain existing)
	_, err = q.Dequeue(context.Background())
	if err != nil {
		t.Errorf("Dequeue after close should still work, got error: %v", err)
	}
}

func TestDrain(t *testing.T) {
	q := NewRequestQueue("test-agent", 10)

	// Enqueue 5 requests
	for i := 0; i < 5; i++ {
		req := &Request{
			ID:       fmt.Sprintf("req-%d", i),
			Input:    []byte("{}"),
			ResponseCh: make(chan *Response, 1),
		}
		q.Enqueue(context.Background(), req)
	}

	// Drain
	drained := q.Drain()

	// Should return all 5 pending requests
	if len(drained) != 5 {
		t.Errorf("Expected 5 drained requests, got %d", len(drained))
	}

	// Pending should be 0
	if q.PendingTasks() != 0 {
		t.Errorf("After drain, pending should be 0, got %d", q.PendingTasks())
	}

	// Queue should be closed after drain
	req := &Request{
		ID:       "req-new",
		Input:    []byte("{}"),
		ResponseCh: make(chan *Response, 1),
	}
	err := q.Enqueue(context.Background(), req)
	if err != ErrQueueClosed {
		t.Error("Queue should be closed after drain")
	}
}

func TestEnqueue_Concurrent(t *testing.T) {
	q := NewRequestQueue("test-agent", 100)

	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// 100 concurrent enqueues
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			req := &Request{
				ID:       fmt.Sprintf("req-%d", idx),
				Input:    []byte("{}"),
				ResponseCh: make(chan *Response, 1),
			}
			err := q.Enqueue(context.Background(), req)
			if err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// All should succeed (under capacity)
	errCount := 0
	for err := range errors {
		t.Logf("Enqueue error: %v", err)
		errCount++
	}

	if errCount > 0 {
		t.Errorf("Expected 0 errors, got %d", errCount)
	}

	// Should have exactly 100 pending
	if q.PendingTasks() != 100 {
		t.Errorf("Expected 100 pending, got %d", q.PendingTasks())
	}
}

func TestQueueLength(t *testing.T) {
	q := NewRequestQueue("test-agent", 10)

	// Enqueue 3
	for i := 0; i < 3; i++ {
		req := &Request{
			ID:       fmt.Sprintf("req-%d", i),
			Input:    []byte("{}"),
			ResponseCh: make(chan *Response, 1),
		}
		q.Enqueue(context.Background(), req)
	}

	// Dequeue 2 (move to processing)
	q.Dequeue(context.Background())
	q.Dequeue(context.Background())

	// Queue length = pending + processing = 1 + 2 = 3
	length := q.QueueLength()
	if length != 3 {
		t.Errorf("Expected queue length 3 (1 pending + 2 processing), got %d", length)
	}
}
