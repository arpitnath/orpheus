package scaling

import (
	"testing"
	"time"

	"orpheus/daemon/pkg/config"
)

func TestNewAgentWorker(t *testing.T) {
	cfg := &config.AgentConfig{
		Name:    "test-agent",
		Runtime: "python3",
	}

	worker, err := newAgentWorker("worker-1", "test-agent", cfg, "/path/to/image")
	if err != nil {
		t.Fatalf("newAgentWorker failed: %v", err)
	}

	if worker == nil {
		t.Fatal("newAgentWorker returned nil")
	}

	if worker.ID() != "worker-1" {
		t.Errorf("Expected ID=worker-1, got %s", worker.ID())
	}

	if worker.AgentID() != "test-agent" {
		t.Errorf("Expected AgentID=test-agent, got %s", worker.AgentID())
	}
}

func TestAgentWorker_ID(t *testing.T) {
	cfg := &config.AgentConfig{Name: "test"}
	worker, _ := newAgentWorker("my-worker-id", "test", cfg, "/path")

	if worker.ID() != "my-worker-id" {
		t.Errorf("Expected ID=my-worker-id, got %s", worker.ID())
	}
}

func TestAgentWorker_AgentID(t *testing.T) {
	cfg := &config.AgentConfig{Name: "my-agent"}
	worker, _ := newAgentWorker("worker-1", "my-agent", cfg, "/path")

	if worker.AgentID() != "my-agent" {
		t.Errorf("Expected AgentID=my-agent, got %s", worker.AgentID())
	}
}

func TestAgentWorker_IsIdle_Initial(t *testing.T) {
	cfg := &config.AgentConfig{Name: "test"}
	worker, _ := newAgentWorker("worker-1", "test", cfg, "/path")

	// New worker should be idle
	if !worker.IsIdle() {
		t.Error("New worker should be idle")
	}
}

func TestAgentWorker_Health_Initial(t *testing.T) {
	cfg := &config.AgentConfig{Name: "test"}
	worker, _ := newAgentWorker("worker-1", "test", cfg, "/path")

	// New worker should be healthy
	if worker.Health() != HealthHealthy {
		t.Errorf("New worker should be healthy, got %v", worker.Health())
	}
}

func TestAgentWorker_LastUsed(t *testing.T) {
	cfg := &config.AgentConfig{Name: "test"}

	before := time.Now()
	worker, _ := newAgentWorker("worker-1", "test", cfg, "/path")
	after := time.Now()

	lastUsed := worker.LastUsed()

	if lastUsed.Before(before) || lastUsed.After(after) {
		t.Errorf("LastUsed (%v) should be between %v and %v", lastUsed, before, after)
	}
}

func TestAgentWorker_Shutdown(t *testing.T) {
	cfg := &config.AgentConfig{Name: "test"}
	worker, _ := newAgentWorker("worker-1", "test", cfg, "/path")

	err := worker.Shutdown(nil)
	if err != nil {
		t.Errorf("Shutdown should not error: %v", err)
	}

	// After shutdown, health should be unhealthy
	if worker.Health() != HealthUnhealthy {
		t.Errorf("After shutdown, health should be unhealthy, got %v", worker.Health())
	}
}

func TestAgentWorker_Shutdown_Idempotent(t *testing.T) {
	cfg := &config.AgentConfig{Name: "test"}
	worker, _ := newAgentWorker("worker-1", "test", cfg, "/path")

	// First shutdown
	worker.Shutdown(nil)

	// Second shutdown should not error
	err := worker.Shutdown(nil)
	if err != nil {
		t.Errorf("Second shutdown should not error: %v", err)
	}
}

func TestHealthStatus_String(t *testing.T) {
	tests := []struct {
		status   HealthStatus
		expected string
	}{
		{HealthUnknown, "unknown"},
		{HealthHealthy, "healthy"},
		{HealthUnhealthy, "unhealthy"},
		{HealthDegraded, "degraded"},
		{HealthStatus(99), "unknown"}, // Invalid value
	}

	for _, tc := range tests {
		result := tc.status.String()
		if result != tc.expected {
			t.Errorf("HealthStatus(%d).String() = %s, expected %s", tc.status, result, tc.expected)
		}
	}
}

func TestNewAgentSpawner(t *testing.T) {
	cfg := &config.AgentConfig{Name: "spawner-test"}

	spawner := NewAgentSpawner(cfg, "/image/path")

	if spawner == nil {
		t.Fatal("NewAgentSpawner returned nil")
	}

	if spawner.agentID != "spawner-test" {
		t.Errorf("Expected agentID=spawner-test, got %s", spawner.agentID)
	}

	if spawner.imagePath != "/image/path" {
		t.Errorf("Expected imagePath=/image/path, got %s", spawner.imagePath)
	}
}

func TestAgentSpawner_SpawnWorker(t *testing.T) {
	cfg := &config.AgentConfig{Name: "spawn-test"}
	spawner := NewAgentSpawner(cfg, "/image/path")

	worker, err := spawner.SpawnWorker(nil, "spawn-test")
	if err != nil {
		t.Fatalf("SpawnWorker failed: %v", err)
	}

	if worker == nil {
		t.Fatal("SpawnWorker returned nil")
	}

	// Verify worker ID format
	if worker.ID() != "spawn-test-worker-1" {
		t.Errorf("Expected worker ID spawn-test-worker-1, got %s", worker.ID())
	}
}

func TestAgentSpawner_SpawnWorker_UniqueIDs(t *testing.T) {
	cfg := &config.AgentConfig{Name: "unique-test"}
	spawner := NewAgentSpawner(cfg, "/image/path")

	worker1, _ := spawner.SpawnWorker(nil, "unique-test")
	worker2, _ := spawner.SpawnWorker(nil, "unique-test")
	worker3, _ := spawner.SpawnWorker(nil, "unique-test")

	ids := map[string]bool{
		worker1.ID(): true,
		worker2.ID(): true,
		worker3.ID(): true,
	}

	if len(ids) != 3 {
		t.Error("Worker IDs should be unique")
	}
}

func TestAgentSpawner_KillWorker(t *testing.T) {
	cfg := &config.AgentConfig{Name: "kill-test"}
	spawner := NewAgentSpawner(cfg, "/image/path")

	// KillWorker is a no-op for in-process workers
	err := spawner.KillWorker(nil, "any-worker-id")
	if err != nil {
		t.Errorf("KillWorker should not error: %v", err)
	}
}

func TestScalingPolicy(t *testing.T) {
	policy := ScalingPolicy{
		MinWorkers:         1,
		MaxWorkers:         10,
		TargetUtilization:  2.0,
		ScaleUpThreshold:   3.0,
		ScaleDownThreshold: 0.5,
		ScaleUpDelay:       30 * time.Second,
		ScaleDownDelay:     60 * time.Second,
		IdleTimeout:        5 * time.Minute,
	}

	if policy.MinWorkers != 1 {
		t.Errorf("MinWorkers mismatch")
	}
	if policy.MaxWorkers != 10 {
		t.Errorf("MaxWorkers mismatch")
	}
	if policy.TargetUtilization != 2.0 {
		t.Errorf("TargetUtilization mismatch")
	}
}

func TestPoolStats(t *testing.T) {
	now := time.Now()

	stats := PoolStats{
		AgentID:       "stats-test",
		TotalWorkers:  5,
		IdleWorkers:   3,
		BusyWorkers:   2,
		DesiredSize:   5,
		LastScaleTime: now,
	}

	if stats.AgentID != "stats-test" {
		t.Errorf("AgentID mismatch")
	}
	if stats.TotalWorkers != 5 {
		t.Errorf("TotalWorkers mismatch")
	}
	if stats.IdleWorkers != 3 {
		t.Errorf("IdleWorkers mismatch")
	}
	if stats.BusyWorkers != 2 {
		t.Errorf("BusyWorkers mismatch")
	}
}

func TestResult(t *testing.T) {
	result := Result{
		Status:   "success",
		Output:   map[string]interface{}{"key": "value"},
		Error:    "",
		Stderr:   "",
		ExitCode: 0,
		Duration: 100 * time.Millisecond,
	}

	if result.Status != "success" {
		t.Errorf("Status mismatch")
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode mismatch")
	}
	if result.Duration != 100*time.Millisecond {
		t.Errorf("Duration mismatch")
	}
}
