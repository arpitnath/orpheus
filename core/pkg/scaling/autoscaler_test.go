package scaling

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestNewAutoscaler(t *testing.T) {
	a := NewAutoscaler()

	if a == nil {
		t.Fatal("NewAutoscaler returned nil")
	}

	if a.interval != DefaultScalingInterval {
		t.Errorf("Expected interval=%v, got %v", DefaultScalingInterval, a.interval)
	}
}

func TestNewAutoscalerWithInterval(t *testing.T) {
	interval := 10 * time.Second
	a := NewAutoscalerWithInterval(interval)

	if a == nil {
		t.Fatal("NewAutoscalerWithInterval returned nil")
	}

	if a.interval != interval {
		t.Errorf("Expected interval=%v, got %v", interval, a.interval)
	}
}

func TestCalculateDesiredSize_ScaleUp(t *testing.T) {
	a := NewAutoscaler()

	policy := ScalingPolicy{
		MinWorkers:         1,
		MaxWorkers:         10,
		TargetUtilization:  2.0,
		ScaleUpThreshold:   3.0, // Scale up when > 3 tasks per worker
		ScaleDownThreshold: 0.5,
	}

	// Current: 2 workers, 10 tasks (pending + processing)
	// Utilization: 10/2 = 5.0 (> 3.0 threshold)
	// Target: ceil(10 / 2.0) = 5 workers
	desired := a.calculateDesiredSize(2, 5, 5, policy)

	if desired != 5 {
		t.Errorf("Expected scale up to 5 workers, got %d", desired)
	}
}

func TestCalculateDesiredSize_ScaleDown(t *testing.T) {
	a := NewAutoscaler()

	policy := ScalingPolicy{
		MinWorkers:         1,
		MaxWorkers:         10,
		TargetUtilization:  2.0,
		ScaleUpThreshold:   3.0,
		ScaleDownThreshold: 0.5, // Scale down when < 0.5 tasks per worker
	}

	// Current: 10 workers, 2 tasks
	// Utilization: 2/10 = 0.2 (< 0.5 threshold)
	// Target: ceil(2 / 2.0) = 1 worker
	desired := a.calculateDesiredSize(10, 1, 1, policy)

	if desired != 1 {
		t.Errorf("Expected scale down to 1 worker, got %d", desired)
	}
}

func TestCalculateDesiredSize_Stable(t *testing.T) {
	a := NewAutoscaler()

	policy := ScalingPolicy{
		MinWorkers:         1,
		MaxWorkers:         10,
		TargetUtilization:  2.0,
		ScaleUpThreshold:   3.0,
		ScaleDownThreshold: 0.5,
	}

	// Current: 5 workers, 10 tasks
	// Utilization: 10/5 = 2.0 (between 0.5 and 3.0)
	// Should stay stable
	desired := a.calculateDesiredSize(5, 5, 5, policy)

	if desired != 5 {
		t.Errorf("Expected stable at 5 workers, got %d", desired)
	}
}

func TestCalculateDesiredSize_MinBounds(t *testing.T) {
	a := NewAutoscaler()

	policy := ScalingPolicy{
		MinWorkers:         3, // Minimum 3 workers
		MaxWorkers:         10,
		TargetUtilization:  2.0,
		ScaleUpThreshold:   3.0,
		ScaleDownThreshold: 0.5,
	}

	// Current: 5 workers, 1 task
	// Utilization: 1/5 = 0.2 (< 0.5, triggers scale down)
	// Target: ceil(1 / 2.0) = 1, but min is 3
	desired := a.calculateDesiredSize(5, 0, 1, policy)

	if desired < policy.MinWorkers {
		t.Errorf("Should clamp to min=%d, got %d", policy.MinWorkers, desired)
	}
}

func TestCalculateDesiredSize_MaxBounds(t *testing.T) {
	a := NewAutoscaler()

	policy := ScalingPolicy{
		MinWorkers:         1,
		MaxWorkers:         5, // Maximum 5 workers
		TargetUtilization:  2.0,
		ScaleUpThreshold:   3.0,
		ScaleDownThreshold: 0.5,
	}

	// Current: 3 workers, 30 tasks
	// Utilization: 30/3 = 10.0 (> 3.0, triggers scale up)
	// Target: ceil(30 / 2.0) = 15, but max is 5
	desired := a.calculateDesiredSize(3, 20, 10, policy)

	if desired > policy.MaxWorkers {
		t.Errorf("Should clamp to max=%d, got %d", policy.MaxWorkers, desired)
	}
}

func TestCalculateDesiredSize_ZeroWorkers(t *testing.T) {
	a := NewAutoscaler()

	policy := ScalingPolicy{
		MinWorkers:         2,
		MaxWorkers:         10,
		TargetUtilization:  2.0,
		ScaleUpThreshold:   3.0,
		ScaleDownThreshold: 0.5,
	}

	// Edge case: 0 workers - should bootstrap to minimum
	desired := a.calculateDesiredSize(0, 5, 0, policy)

	if desired != policy.MinWorkers {
		t.Errorf("With 0 workers, should bootstrap to min=%d, got %d", policy.MinWorkers, desired)
	}
}

func TestRegisterPool(t *testing.T) {
	a := NewAutoscaler()

	pool := &mockWorkerPool{size: 2}
	policy := ScalingPolicy{MinWorkers: 1, MaxWorkers: 10}

	a.RegisterPool("test-agent", pool, policy)

	a.mu.RLock()
	defer a.mu.RUnlock()

	if _, exists := a.pools["test-agent"]; !exists {
		t.Error("Pool should be registered")
	}

	if _, exists := a.policies["test-agent"]; !exists {
		t.Error("Policy should be registered")
	}
}

func TestUnregisterPool(t *testing.T) {
	a := NewAutoscaler()

	pool := &mockWorkerPool{size: 2}
	policy := ScalingPolicy{MinWorkers: 1, MaxWorkers: 10}

	a.RegisterPool("test-agent", pool, policy)
	a.UnregisterPool("test-agent")

	a.mu.RLock()
	defer a.mu.RUnlock()

	if _, exists := a.pools["test-agent"]; exists {
		t.Error("Pool should be unregistered")
	}
}

func TestRegisterQueueMetrics(t *testing.T) {
	a := NewAutoscaler()

	metrics := &mockQueueMetrics{pending: 5, processing: 3}

	a.RegisterQueueMetrics("test-agent", metrics)

	a.mu.RLock()
	defer a.mu.RUnlock()

	if _, exists := a.metrics["test-agent"]; !exists {
		t.Error("Metrics should be registered")
	}
}

func TestStartStop(t *testing.T) {
	a := NewAutoscalerWithInterval(10 * time.Millisecond)

	ctx := context.Background()

	// Start
	err := a.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Verify running
	a.mu.RLock()
	running := a.running
	a.mu.RUnlock()

	if !running {
		t.Error("Should be running after Start")
	}

	// Stop
	err = a.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	a.mu.RLock()
	running = a.running
	a.mu.RUnlock()

	if running {
		t.Error("Should not be running after Stop")
	}
}

func TestStartIdempotent(t *testing.T) {
	a := NewAutoscalerWithInterval(100 * time.Millisecond)

	ctx := context.Background()

	// Start twice - should not error
	a.Start(ctx)
	err := a.Start(ctx)

	if err != nil {
		t.Errorf("Second Start should not error, got: %v", err)
	}

	a.Stop()
}

func TestStopIdempotent(t *testing.T) {
	a := NewAutoscaler()

	// Stop without starting - should not error
	err := a.Stop()
	if err != nil {
		t.Errorf("Stop without Start should not error, got: %v", err)
	}
}

// mockWorkerPool implements WorkerPool for testing
type mockWorkerPool struct {
	agentID     string
	size        int
	desiredSize int
	mu          sync.Mutex
}

func (m *mockWorkerPool) AgentID() string {
	return m.agentID
}

func (m *mockWorkerPool) Size() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.size
}

func (m *mockWorkerPool) DesiredSize() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.desiredSize
}

func (m *mockWorkerPool) SetDesiredSize(size int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.desiredSize = size
}

func (m *mockWorkerPool) GetIdleWorker(ctx context.Context) (Worker, error) {
	return nil, nil
}

func (m *mockWorkerPool) ReturnWorker(worker Worker) {}

func (m *mockWorkerPool) AcquireForSession(ctx context.Context, sessionID string) (Worker, error) {
	return nil, nil
}

func (m *mockWorkerPool) ClearSession(sessionID string) {}

func (m *mockWorkerPool) GetStats() PoolStats {
	return PoolStats{TotalWorkers: m.size}
}

func (m *mockWorkerPool) Shutdown(ctx context.Context) error {
	return nil
}

// mockQueueMetrics implements QueueMetrics for testing
type mockQueueMetrics struct {
	pending    int
	processing int
}

func (m *mockQueueMetrics) PendingTasks() int {
	return m.pending
}

func (m *mockQueueMetrics) ProcessingTasks() int {
	return m.processing
}

func (m *mockQueueMetrics) QueueLength() int {
	return m.pending + m.processing
}
