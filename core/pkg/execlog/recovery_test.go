package execlog

import (
	"testing"
	"time"
)

func TestDetectAndMarkCrashed_Empty(t *testing.T) {
	tmpDir := t.TempDir()

	// No databases - should return empty map
	crashed, err := DetectAndMarkCrashed(tmpDir)
	if err != nil {
		t.Fatalf("DetectAndMarkCrashed failed: %v", err)
	}

	if len(crashed) != 0 {
		t.Errorf("Expected empty map, got %d agents", len(crashed))
	}
}

func TestDetectAndMarkCrashed_NoCrashes(t *testing.T) {
	tmpDir := t.TempDir()

	writerCacheMu.Lock()
	writerCache = make(map[string]*Writer)
	writerCacheMu.Unlock()

	// Create database with completed requests only
	writer, _ := NewWriter(tmpDir, "agent-a")
	writer.Log(&Event{Timestamp: time.Now(), RequestID: "req-1", State: StateQueued})
	writer.Log(&Event{Timestamp: time.Now(), RequestID: "req-1", State: StateStarted})
	writer.Log(&Event{Timestamp: time.Now(), RequestID: "req-1", State: StateCompleted})
	writer.Close()

	crashed, err := DetectAndMarkCrashed(tmpDir)
	if err != nil {
		t.Fatalf("DetectAndMarkCrashed failed: %v", err)
	}

	if len(crashed) != 0 {
		t.Errorf("Expected no crashed requests, got %d", len(crashed))
	}
}

func TestDetectAndMarkCrashed_WithCrashes(t *testing.T) {
	tmpDir := t.TempDir()

	writerCacheMu.Lock()
	writerCache = make(map[string]*Writer)
	writerCacheMu.Unlock()

	// Create database with a request that STARTED but never finished
	// Then manually add CRASHED state to simulate recovery detection
	writer, _ := NewWriter(tmpDir, "crash-agent")

	workerID := "worker-1"

	// This request started but was already marked as crashed
	writer.Log(&Event{Timestamp: time.Now(), RequestID: "crashed-req", State: StateQueued})
	writer.Log(&Event{Timestamp: time.Now(), RequestID: "crashed-req", State: StateStarted, WorkerID: &workerID})
	writer.Log(&Event{Timestamp: time.Now(), RequestID: "crashed-req", State: StateCrashed, WorkerID: &workerID})
	writer.Close()

	crashed, err := DetectAndMarkCrashed(tmpDir)
	if err != nil {
		t.Fatalf("DetectAndMarkCrashed failed: %v", err)
	}

	// Should find the crashed request
	if len(crashed) == 0 {
		t.Error("Expected to find crashed requests")
	}

	if agentCrashed, ok := crashed["crash-agent"]; ok {
		if len(agentCrashed) == 0 {
			t.Error("Expected crashed requests for crash-agent")
		}
	}
}

func TestDetectAndMarkCrashed_MultiAgent(t *testing.T) {
	tmpDir := t.TempDir()

	writerCacheMu.Lock()
	writerCache = make(map[string]*Writer)
	writerCacheMu.Unlock()

	workerID := "worker-1"

	// Agent 1: has crashed request
	writer1, _ := NewWriter(tmpDir, "agent-1")
	writer1.Log(&Event{Timestamp: time.Now(), RequestID: "agent1-req", State: StateQueued})
	writer1.Log(&Event{Timestamp: time.Now(), RequestID: "agent1-req", State: StateStarted, WorkerID: &workerID})
	writer1.Log(&Event{Timestamp: time.Now(), RequestID: "agent1-req", State: StateCrashed, WorkerID: &workerID})
	writer1.Close()

	// Agent 2: no crashes (all completed)
	writer2, _ := NewWriter(tmpDir, "agent-2")
	writer2.Log(&Event{Timestamp: time.Now(), RequestID: "agent2-req", State: StateQueued})
	writer2.Log(&Event{Timestamp: time.Now(), RequestID: "agent2-req", State: StateStarted})
	writer2.Log(&Event{Timestamp: time.Now(), RequestID: "agent2-req", State: StateCompleted})
	writer2.Close()

	crashed, err := DetectAndMarkCrashed(tmpDir)
	if err != nil {
		t.Fatalf("DetectAndMarkCrashed failed: %v", err)
	}

	// Only agent-1 should have crashed requests
	if _, ok := crashed["agent-1"]; !ok {
		t.Error("Expected crashed requests for agent-1")
	}

	// agent-2 should NOT be in the map (no crashes)
	if _, ok := crashed["agent-2"]; ok {
		t.Error("agent-2 should not have crashed requests")
	}
}

func TestCrashedRequest_Fields(t *testing.T) {
	req := CrashedRequest{
		RequestID: "test-req",
		AgentName: "test-agent",
		WorkerID:  "worker-123",
		SessionID: nil,
		StartedAt: time.Now(),
	}

	if req.RequestID != "test-req" {
		t.Errorf("RequestID mismatch")
	}

	if req.AgentName != "test-agent" {
		t.Errorf("AgentName mismatch")
	}

	if req.WorkerID != "worker-123" {
		t.Errorf("WorkerID mismatch")
	}

	if req.SessionID != nil {
		t.Error("SessionID should be nil")
	}
}

func TestExecLogFilters(t *testing.T) {
	filters := ExecLogFilters{
		Status:    StateCompleted,
		WorkerID:  "worker-1",
		SessionID: "session-abc",
		StartTime: time.Now().UnixNano(),
		EndTime:   time.Now().Add(time.Hour).UnixNano(),
		Limit:     100,
		Offset:    0,
	}

	if filters.Status != StateCompleted {
		t.Error("Status mismatch")
	}

	if filters.WorkerID != "worker-1" {
		t.Error("WorkerID mismatch")
	}

	if filters.Limit != 100 {
		t.Error("Limit mismatch")
	}
}

func TestExecLogStats(t *testing.T) {
	stats := ExecLogStats{
		Queued:       10,
		Started:      10,
		Completed:    8,
		Failed:       1,
		Crashed:      1,
		Total:        30,
		AvgDuration:  150.5,
		MinDuration:  50,
		MaxDuration:  500,
		SuccessRate:  80.0,
		ErrorRate:    6.67,
		CrashRate:    3.33,
		HealthStatus: "degraded",
	}

	if stats.Total != 30 {
		t.Error("Total mismatch")
	}

	if stats.HealthStatus != "degraded" {
		t.Error("HealthStatus mismatch")
	}
}
