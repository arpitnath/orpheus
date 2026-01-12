package scaling

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// mockWorker implements Worker for testing
type mockWorker struct {
	id       string
	agentID  string
	health   atomic.Int32
	idle     atomic.Bool
	lastUsed atomic.Int64
	shutdown atomic.Bool
}

func newMockWorker(id, agentID string) *mockWorker {
	w := &mockWorker{
		id:      id,
		agentID: agentID,
	}
	w.health.Store(int32(HealthHealthy))
	w.idle.Store(true)
	w.lastUsed.Store(time.Now().UnixNano())
	return w
}

func (w *mockWorker) ID() string                                         { return w.id }
func (w *mockWorker) AgentID() string                                    { return w.agentID }
func (w *mockWorker) IsIdle() bool                                       { return w.idle.Load() }
func (w *mockWorker) Health() HealthStatus                               { return HealthStatus(w.health.Load()) }
func (w *mockWorker) LastUsed() time.Time                                { return time.Unix(0, w.lastUsed.Load()) }
func (w *mockWorker) Execute(ctx context.Context, input []byte) (*Result, error) {
	w.idle.Store(false)
	defer func() {
		w.idle.Store(true)
		w.lastUsed.Store(time.Now().UnixNano())
	}()
	return &Result{Status: "success"}, nil
}
func (w *mockWorker) Shutdown(ctx context.Context) error {
	w.shutdown.Store(true)
	w.health.Store(int32(HealthUnhealthy))
	return nil
}

// mockSpawner implements WorkerSpawner for testing
type mockSpawner struct {
	agentID string
	counter atomic.Int64
	mu      sync.Mutex
	workers map[string]*mockWorker
}

func newMockSpawner(agentID string) *mockSpawner {
	return &mockSpawner{
		agentID: agentID,
		workers: make(map[string]*mockWorker),
	}
}

func (s *mockSpawner) SpawnWorker(ctx context.Context, agentID string) (Worker, error) {
	count := s.counter.Add(1)
	id := agentID + "-mock-" + string(rune('0'+count))
	worker := newMockWorker(id, agentID)
	s.mu.Lock()
	s.workers[id] = worker
	s.mu.Unlock()
	return worker, nil
}

func (s *mockSpawner) KillWorker(ctx context.Context, workerID string) error {
	s.mu.Lock()
	delete(s.workers, workerID)
	s.mu.Unlock()
	return nil
}

func TestNewWorkerPool(t *testing.T) {
	spawner := newMockSpawner("test-agent")
	policy := ScalingPolicy{
		MinWorkers: 2,
		MaxWorkers: 10,
	}

	pool := NewWorkerPool("test-agent", spawner, policy)
	defer pool.Shutdown(context.Background())

	if pool == nil {
		t.Fatal("NewWorkerPool returned nil")
	}

	if pool.AgentID() != "test-agent" {
		t.Errorf("Expected AgentID=test-agent, got %s", pool.AgentID())
	}

	// Should have min workers
	if pool.Size() != 2 {
		t.Errorf("Expected Size=2 (min workers), got %d", pool.Size())
	}
}

func TestWorkerPool_AgentID(t *testing.T) {
	spawner := newMockSpawner("my-agent")
	policy := ScalingPolicy{MinWorkers: 1, MaxWorkers: 5}

	pool := NewWorkerPool("my-agent", spawner, policy)
	defer pool.Shutdown(context.Background())

	if pool.AgentID() != "my-agent" {
		t.Errorf("Expected AgentID=my-agent, got %s", pool.AgentID())
	}
}

func TestWorkerPool_Size(t *testing.T) {
	spawner := newMockSpawner("test")
	policy := ScalingPolicy{MinWorkers: 3, MaxWorkers: 10}

	pool := NewWorkerPool("test", spawner, policy)
	defer pool.Shutdown(context.Background())

	if pool.Size() != 3 {
		t.Errorf("Expected Size=3, got %d", pool.Size())
	}
}

func TestWorkerPool_DesiredSize(t *testing.T) {
	spawner := newMockSpawner("test")
	policy := ScalingPolicy{MinWorkers: 2, MaxWorkers: 10}

	pool := NewWorkerPool("test", spawner, policy)
	defer pool.Shutdown(context.Background())

	// Initial desired size should match min
	if pool.DesiredSize() != 2 {
		t.Errorf("Expected DesiredSize=2, got %d", pool.DesiredSize())
	}
}

func TestWorkerPool_SetDesiredSize(t *testing.T) {
	spawner := newMockSpawner("test")
	policy := ScalingPolicy{MinWorkers: 1, MaxWorkers: 5}

	pool := NewWorkerPool("test", spawner, policy)
	defer pool.Shutdown(context.Background())

	pool.SetDesiredSize(3)

	if pool.DesiredSize() != 3 {
		t.Errorf("Expected DesiredSize=3, got %d", pool.DesiredSize())
	}
}

func TestWorkerPool_SetDesiredSize_ClampMin(t *testing.T) {
	spawner := newMockSpawner("test")
	policy := ScalingPolicy{MinWorkers: 2, MaxWorkers: 10}

	pool := NewWorkerPool("test", spawner, policy)
	defer pool.Shutdown(context.Background())

	// Try to set below min
	pool.SetDesiredSize(0)

	if pool.DesiredSize() != 2 {
		t.Errorf("Should clamp to min=2, got %d", pool.DesiredSize())
	}
}

func TestWorkerPool_SetDesiredSize_ClampMax(t *testing.T) {
	spawner := newMockSpawner("test")
	policy := ScalingPolicy{MinWorkers: 1, MaxWorkers: 5}

	pool := NewWorkerPool("test", spawner, policy)
	defer pool.Shutdown(context.Background())

	// Try to set above max
	pool.SetDesiredSize(100)

	if pool.DesiredSize() != 5 {
		t.Errorf("Should clamp to max=5, got %d", pool.DesiredSize())
	}
}

func TestWorkerPool_GetIdleWorker(t *testing.T) {
	spawner := newMockSpawner("test")
	policy := ScalingPolicy{MinWorkers: 2, MaxWorkers: 10}

	pool := NewWorkerPool("test", spawner, policy)
	defer pool.Shutdown(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	worker, err := pool.GetIdleWorker(ctx)
	if err != nil {
		t.Fatalf("GetIdleWorker failed: %v", err)
	}

	if worker == nil {
		t.Fatal("GetIdleWorker returned nil")
	}
}

func TestWorkerPool_GetIdleWorker_ContextCancel(t *testing.T) {
	spawner := newMockSpawner("test")
	policy := ScalingPolicy{MinWorkers: 1, MaxWorkers: 10}

	pool := NewWorkerPool("test", spawner, policy)
	defer pool.Shutdown(context.Background())

	// Get the only worker
	ctx1 := context.Background()
	pool.GetIdleWorker(ctx1)

	// Now try with cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := pool.GetIdleWorker(ctx)
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got: %v", err)
	}
}

func TestWorkerPool_ReturnWorker(t *testing.T) {
	spawner := newMockSpawner("test")
	policy := ScalingPolicy{MinWorkers: 1, MaxWorkers: 10}

	pool := NewWorkerPool("test", spawner, policy)
	defer pool.Shutdown(context.Background())

	ctx := context.Background()

	// Get a worker
	worker, _ := pool.GetIdleWorker(ctx)

	// Return it
	pool.ReturnWorker(worker)

	// Should be able to get it again
	worker2, err := pool.GetIdleWorker(ctx)
	if err != nil {
		t.Fatalf("GetIdleWorker after return failed: %v", err)
	}

	if worker2 == nil {
		t.Error("Should get worker after returning")
	}
}

func TestWorkerPool_ReturnWorker_Nil(t *testing.T) {
	spawner := newMockSpawner("test")
	policy := ScalingPolicy{MinWorkers: 1, MaxWorkers: 5}

	pool := NewWorkerPool("test", spawner, policy)
	defer pool.Shutdown(context.Background())

	// Should not panic
	pool.ReturnWorker(nil)
}

func TestWorkerPool_GetStats(t *testing.T) {
	spawner := newMockSpawner("test")
	policy := ScalingPolicy{MinWorkers: 3, MaxWorkers: 10}

	pool := NewWorkerPool("test", spawner, policy)
	defer pool.Shutdown(context.Background())

	stats := pool.GetStats()

	if stats.AgentID != "test" {
		t.Errorf("Expected AgentID=test, got %s", stats.AgentID)
	}

	if stats.TotalWorkers != 3 {
		t.Errorf("Expected TotalWorkers=3, got %d", stats.TotalWorkers)
	}

	// All should be idle initially
	if stats.IdleWorkers != 3 {
		t.Errorf("Expected IdleWorkers=3, got %d", stats.IdleWorkers)
	}

	if stats.BusyWorkers != 0 {
		t.Errorf("Expected BusyWorkers=0, got %d", stats.BusyWorkers)
	}
}

func TestWorkerPool_Shutdown(t *testing.T) {
	spawner := newMockSpawner("test")
	policy := ScalingPolicy{MinWorkers: 2, MaxWorkers: 10}

	pool := NewWorkerPool("test", spawner, policy)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := pool.Shutdown(ctx)
	if err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	// Pool should be empty after shutdown
	if pool.Size() != 0 {
		t.Errorf("After shutdown, size should be 0, got %d", pool.Size())
	}
}

func TestWorkerPool_ClearSession(t *testing.T) {
	spawner := newMockSpawner("test")
	policy := ScalingPolicy{MinWorkers: 1, MaxWorkers: 5}

	pool := NewWorkerPool("test", spawner, policy)
	defer pool.Shutdown(context.Background())

	ctx := context.Background()

	// Acquire for session to create mapping
	pool.AcquireForSession(ctx, "session-123", 0)

	// Verify session exists
	_, exists := pool.GetSessionWorker("session-123")
	if !exists {
		t.Error("Session should exist after acquire")
	}

	// Clear session
	pool.ClearSession("session-123")

	// Verify session cleared
	_, exists = pool.GetSessionWorker("session-123")
	if exists {
		t.Error("Session should not exist after clear")
	}
}

func TestWorkerPool_AcquireForSession_EmptySession(t *testing.T) {
	spawner := newMockSpawner("test")
	policy := ScalingPolicy{MinWorkers: 1, MaxWorkers: 5}

	pool := NewWorkerPool("test", spawner, policy)
	defer pool.Shutdown(context.Background())

	ctx := context.Background()

	// Empty session should behave like GetIdleWorker
	worker, err := pool.AcquireForSession(ctx, "", 0)
	if err != nil {
		t.Fatalf("AcquireForSession with empty session failed: %v", err)
	}

	if worker == nil {
		t.Fatal("Should get worker")
	}
}

func TestWorkerPool_AcquireForSession_FirstRequest(t *testing.T) {
	spawner := newMockSpawner("test")
	policy := ScalingPolicy{MinWorkers: 2, MaxWorkers: 5}

	pool := NewWorkerPool("test", spawner, policy)
	defer pool.Shutdown(context.Background())

	ctx := context.Background()

	// First request creates session mapping
	worker1, err := pool.AcquireForSession(ctx, "session-abc", 0)
	if err != nil {
		t.Fatalf("First AcquireForSession failed: %v", err)
	}

	// Verify session mapped to worker
	workerID, exists := pool.GetSessionWorker("session-abc")
	if !exists {
		t.Error("Session should be mapped after first request")
	}

	if workerID != worker1.ID() {
		t.Errorf("Session should map to acquired worker, got %s vs %s", workerID, worker1.ID())
	}
}

func TestWorkerPool_GetSessionWorker(t *testing.T) {
	spawner := newMockSpawner("test")
	policy := ScalingPolicy{MinWorkers: 1, MaxWorkers: 5}

	pool := NewWorkerPool("test", spawner, policy)
	defer pool.Shutdown(context.Background())

	ctx := context.Background()

	// Acquire to create mapping
	worker, _ := pool.AcquireForSession(ctx, "session-xyz", 0)
	pool.ReturnWorker(worker)

	// Get session worker
	workerID, exists := pool.GetSessionWorker("session-xyz")
	if !exists {
		t.Error("Session should exist")
	}

	if workerID != worker.ID() {
		t.Errorf("Expected workerID=%s, got %s", worker.ID(), workerID)
	}
}

func TestWorkerPool_GetSessionWorker_NotExists(t *testing.T) {
	spawner := newMockSpawner("test")
	policy := ScalingPolicy{MinWorkers: 1, MaxWorkers: 5}

	pool := NewWorkerPool("test", spawner, policy)
	defer pool.Shutdown(context.Background())

	// Non-existent session
	_, exists := pool.GetSessionWorker("no-such-session")
	if exists {
		t.Error("Non-existent session should return false")
	}
}

func TestWorkerPool_Concurrent(t *testing.T) {
	spawner := newMockSpawner("test")
	policy := ScalingPolicy{MinWorkers: 5, MaxWorkers: 20}

	pool := NewWorkerPool("test", spawner, policy)
	defer pool.Shutdown(context.Background())

	ctx := context.Background()
	var wg sync.WaitGroup

	// 10 concurrent requests
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			worker, err := pool.GetIdleWorker(ctx)
			if err == nil && worker != nil {
				// Simulate work
				time.Sleep(10 * time.Millisecond)
				pool.ReturnWorker(worker)
			}
		}()
	}

	wg.Wait()

	// Pool should still be functional
	stats := pool.GetStats()
	if stats.TotalWorkers < policy.MinWorkers {
		t.Errorf("Pool should maintain at least min workers, got %d", stats.TotalWorkers)
	}
}
