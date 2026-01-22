package execlog

import (
	"testing"
	"time"
)

func TestNewReader(t *testing.T) {
	tmpDir := t.TempDir()

	// Clear writer cache and create a database first
	writerCacheMu.Lock()
	writerCache = make(map[string]*Writer)
	writerCacheMu.Unlock()

	writer, _ := NewWriter(tmpDir, "reader-test")
	writer.Close()

	reader, err := NewReader(tmpDir, "reader-test")
	if err != nil {
		t.Fatalf("NewReader failed: %v", err)
	}
	defer reader.Close()

	if reader == nil {
		t.Fatal("NewReader returned nil")
	}
}

func TestGetExecutionLogs_All(t *testing.T) {
	tmpDir := t.TempDir()

	writerCacheMu.Lock()
	writerCache = make(map[string]*Writer)
	writerCacheMu.Unlock()

	writer, _ := NewWriter(tmpDir, "logs-test")

	// Write some events
	for i := 0; i < 5; i++ {
		writer.Log(&Event{
			Timestamp: time.Now(),
			RequestID: "req-" + string(rune('A'+i)),
			State:     StateQueued,
		})
	}
	writer.Close()

	reader, _ := NewReader(tmpDir, "logs-test")
	defer reader.Close()

	filters := &ExecLogFilters{}
	logs, err := reader.GetExecutionLogs(filters)
	if err != nil {
		t.Fatalf("GetExecutionLogs failed: %v", err)
	}

	if len(logs) != 5 {
		t.Errorf("Expected 5 logs, got %d", len(logs))
	}
}

func TestGetExecutionLogs_FilterByStatus(t *testing.T) {
	tmpDir := t.TempDir()

	writerCacheMu.Lock()
	writerCache = make(map[string]*Writer)
	writerCacheMu.Unlock()

	writer, _ := NewWriter(tmpDir, "filter-test")

	// Write events with different states
	writer.Log(&Event{Timestamp: time.Now(), RequestID: "req-1", State: StateQueued})
	writer.Log(&Event{Timestamp: time.Now(), RequestID: "req-2", State: StateStarted})
	writer.Log(&Event{Timestamp: time.Now(), RequestID: "req-3", State: StateCompleted})
	writer.Log(&Event{Timestamp: time.Now(), RequestID: "req-4", State: StateCompleted})
	writer.Close()

	reader, _ := NewReader(tmpDir, "filter-test")
	defer reader.Close()

	filters := &ExecLogFilters{Status: StateCompleted}
	logs, err := reader.GetExecutionLogs(filters)
	if err != nil {
		t.Fatalf("GetExecutionLogs failed: %v", err)
	}

	if len(logs) != 2 {
		t.Errorf("Expected 2 COMPLETED logs, got %d", len(logs))
	}

	for _, log := range logs {
		if log.State != StateCompleted {
			t.Errorf("Expected state COMPLETED, got %s", log.State)
		}
	}
}

func TestGetExecutionLogs_Pagination(t *testing.T) {
	tmpDir := t.TempDir()

	writerCacheMu.Lock()
	writerCache = make(map[string]*Writer)
	writerCacheMu.Unlock()

	writer, _ := NewWriter(tmpDir, "pagination-test")

	// Write 10 events
	for i := 0; i < 10; i++ {
		writer.Log(&Event{
			Timestamp: time.Now().Add(time.Duration(i) * time.Second),
			RequestID: "req-" + string(rune('A'+i)),
			State:     StateQueued,
		})
	}
	writer.Close()

	reader, _ := NewReader(tmpDir, "pagination-test")
	defer reader.Close()

	// Get first 3
	filters := &ExecLogFilters{Limit: 3, Offset: 0}
	logs, err := reader.GetExecutionLogs(filters)
	if err != nil {
		t.Fatalf("GetExecutionLogs failed: %v", err)
	}

	if len(logs) != 3 {
		t.Errorf("Expected 3 logs (limit), got %d", len(logs))
	}

	// Get next 3
	filters = &ExecLogFilters{Limit: 3, Offset: 3}
	logs, err = reader.GetExecutionLogs(filters)
	if err != nil {
		t.Fatalf("GetExecutionLogs with offset failed: %v", err)
	}

	if len(logs) != 3 {
		t.Errorf("Expected 3 logs (offset), got %d", len(logs))
	}
}

func TestGetExecutionLogsCount(t *testing.T) {
	tmpDir := t.TempDir()

	writerCacheMu.Lock()
	writerCache = make(map[string]*Writer)
	writerCacheMu.Unlock()

	writer, _ := NewWriter(tmpDir, "count-test")

	// Write events
	for i := 0; i < 7; i++ {
		writer.Log(&Event{
			Timestamp: time.Now(),
			RequestID: "req-" + string(rune('A'+i)),
			State:     StateQueued,
		})
	}
	writer.Close()

	reader, _ := NewReader(tmpDir, "count-test")
	defer reader.Close()

	count, err := reader.GetExecutionLogsCount(&ExecLogFilters{})
	if err != nil {
		t.Fatalf("GetExecutionLogsCount failed: %v", err)
	}

	if count != 7 {
		t.Errorf("Expected count 7, got %d", count)
	}
}

func TestGetStats(t *testing.T) {
	tmpDir := t.TempDir()

	writerCacheMu.Lock()
	writerCache = make(map[string]*Writer)
	writerCacheMu.Unlock()

	writer, _ := NewWriter(tmpDir, "stats-test")

	// Write events of different states
	writer.Log(&Event{Timestamp: time.Now(), RequestID: "req-1", State: StateQueued})
	writer.Log(&Event{Timestamp: time.Now(), RequestID: "req-1", State: StateStarted})

	var dur1 int64 = 100
	writer.Log(&Event{Timestamp: time.Now(), RequestID: "req-1", State: StateCompleted, DurationMs: &dur1})

	writer.Log(&Event{Timestamp: time.Now(), RequestID: "req-2", State: StateQueued})
	writer.Log(&Event{Timestamp: time.Now(), RequestID: "req-2", State: StateStarted})

	var dur2 int64 = 200
	writer.Log(&Event{Timestamp: time.Now(), RequestID: "req-2", State: StateCompleted, DurationMs: &dur2})

	writer.Log(&Event{Timestamp: time.Now(), RequestID: "req-3", State: StateQueued})
	writer.Log(&Event{Timestamp: time.Now(), RequestID: "req-3", State: StateStarted})

	errMsg := "test error"
	writer.Log(&Event{Timestamp: time.Now(), RequestID: "req-3", State: StateFailed, Error: &errMsg})

	writer.Close()

	reader, _ := NewReader(tmpDir, "stats-test")
	defer reader.Close()

	stats, err := reader.GetStats()
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if stats.Queued != 3 {
		t.Errorf("Expected Queued=3, got %d", stats.Queued)
	}

	if stats.Started != 3 {
		t.Errorf("Expected Started=3, got %d", stats.Started)
	}

	if stats.Completed != 2 {
		t.Errorf("Expected Completed=2, got %d", stats.Completed)
	}

	if stats.Failed != 1 {
		t.Errorf("Expected Failed=1, got %d", stats.Failed)
	}

	if stats.Total != 9 {
		t.Errorf("Expected Total=9, got %d", stats.Total)
	}

	// Check duration stats (100 + 200) / 2 = 150
	if stats.AvgDuration != 150 {
		t.Errorf("Expected AvgDuration=150, got %f", stats.AvgDuration)
	}

	if stats.MinDuration != 100 {
		t.Errorf("Expected MinDuration=100, got %d", stats.MinDuration)
	}

	if stats.MaxDuration != 200 {
		t.Errorf("Expected MaxDuration=200, got %d", stats.MaxDuration)
	}
}

func TestGetStats_Empty(t *testing.T) {
	tmpDir := t.TempDir()

	writerCacheMu.Lock()
	writerCache = make(map[string]*Writer)
	writerCacheMu.Unlock()

	writer, _ := NewWriter(tmpDir, "empty-stats")
	writer.Close()

	reader, _ := NewReader(tmpDir, "empty-stats")
	defer reader.Close()

	stats, err := reader.GetStats()
	if err != nil {
		t.Fatalf("GetStats on empty failed: %v", err)
	}

	if stats.Total != 0 {
		t.Errorf("Expected Total=0, got %d", stats.Total)
	}

	if stats.HealthStatus != "no_data" {
		t.Errorf("Expected HealthStatus='no_data', got %s", stats.HealthStatus)
	}
}

func TestReader_Close(t *testing.T) {
	tmpDir := t.TempDir()

	writerCacheMu.Lock()
	writerCache = make(map[string]*Writer)
	writerCacheMu.Unlock()

	writer, _ := NewWriter(tmpDir, "close-reader")
	writer.Close()

	reader, _ := NewReader(tmpDir, "close-reader")

	err := reader.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestGetCrashedRequests_NoCrashes(t *testing.T) {
	tmpDir := t.TempDir()

	writerCacheMu.Lock()
	writerCache = make(map[string]*Writer)
	writerCacheMu.Unlock()

	writer, _ := NewWriter(tmpDir, "no-crash")

	// Write only completed events (no crashes)
	writer.Log(&Event{Timestamp: time.Now(), RequestID: "req-1", State: StateQueued})
	writer.Log(&Event{Timestamp: time.Now(), RequestID: "req-1", State: StateStarted})
	writer.Log(&Event{Timestamp: time.Now(), RequestID: "req-1", State: StateCompleted})
	writer.Close()

	reader, _ := NewReader(tmpDir, "no-crash")
	defer reader.Close()

	crashed, err := reader.GetCrashedRequests()
	if err != nil {
		t.Fatalf("GetCrashedRequests failed: %v", err)
	}

	if len(crashed) != 0 {
		t.Errorf("Expected 0 crashed requests, got %d", len(crashed))
	}
}
