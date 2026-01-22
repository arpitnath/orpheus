package execlog

import (
	"sync"
	"testing"
	"time"
)

func TestNewWriter(t *testing.T) {
	tmpDir := t.TempDir()

	// Clear writer cache for test isolation
	writerCacheMu.Lock()
	writerCache = make(map[string]*Writer)
	writerCacheMu.Unlock()

	writer, err := NewWriter(tmpDir, "test-agent")
	if err != nil {
		t.Fatalf("NewWriter failed: %v", err)
	}
	defer writer.Close()

	if writer == nil {
		t.Fatal("NewWriter returned nil")
	}

	if writer.db == nil {
		t.Error("Writer db should not be nil")
	}
}

func TestNewWriter_Cached(t *testing.T) {
	tmpDir := t.TempDir()

	// Clear writer cache
	writerCacheMu.Lock()
	writerCache = make(map[string]*Writer)
	writerCacheMu.Unlock()

	writer1, err := NewWriter(tmpDir, "cached-agent")
	if err != nil {
		t.Fatalf("First NewWriter failed: %v", err)
	}

	writer2, err := NewWriter(tmpDir, "cached-agent")
	if err != nil {
		t.Fatalf("Second NewWriter failed: %v", err)
	}

	// Should return the same cached instance
	if writer1 != writer2 {
		t.Error("NewWriter should return cached instance for same agent")
	}
}

func TestLog_WritesEvent(t *testing.T) {
	tmpDir := t.TempDir()

	// Clear cache
	writerCacheMu.Lock()
	writerCache = make(map[string]*Writer)
	writerCacheMu.Unlock()

	writer, err := NewWriter(tmpDir, "log-test")
	if err != nil {
		t.Fatalf("NewWriter failed: %v", err)
	}
	defer writer.Close()

	event := &Event{
		Timestamp: time.Now(),
		RequestID: "req-123",
		State:     StateQueued,
	}

	err = writer.Log(event)
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	// Verify by querying
	var count int
	err = writer.db.QueryRow("SELECT COUNT(*) FROM events WHERE request_id = ?", "req-123").Scan(&count)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if count != 1 {
		t.Errorf("Expected 1 event, got %d", count)
	}
}

func TestLog_AllStates(t *testing.T) {
	tmpDir := t.TempDir()

	writerCacheMu.Lock()
	writerCache = make(map[string]*Writer)
	writerCacheMu.Unlock()

	writer, err := NewWriter(tmpDir, "states-test")
	if err != nil {
		t.Fatalf("NewWriter failed: %v", err)
	}
	defer writer.Close()

	states := []string{StateQueued, StateStarted, StateCompleted, StateFailed, StateCrashed}

	for i, state := range states {
		workerID := "worker-1"
		sessionID := "session-abc"
		var durationMs int64 = 100
		errMsg := "test error"

		event := &Event{
			Timestamp: time.Now(),
			RequestID: "req-" + state,
			State:     state,
			WorkerID:  &workerID,
			SessionID: &sessionID,
		}

		// Add optional fields for terminal states
		if state == StateCompleted || state == StateFailed {
			event.DurationMs = &durationMs
		}
		if state == StateFailed {
			event.Error = &errMsg
		}

		err := writer.Log(event)
		if err != nil {
			t.Errorf("Log state %s (index %d) failed: %v", state, i, err)
		}
	}

	// Verify all states written
	var count int
	writer.db.QueryRow("SELECT COUNT(*) FROM events").Scan(&count)
	if count != len(states) {
		t.Errorf("Expected %d events, got %d", len(states), count)
	}
}

func TestLog_Concurrent(t *testing.T) {
	tmpDir := t.TempDir()

	writerCacheMu.Lock()
	writerCache = make(map[string]*Writer)
	writerCacheMu.Unlock()

	writer, err := NewWriter(tmpDir, "concurrent-test")
	if err != nil {
		t.Fatalf("NewWriter failed: %v", err)
	}
	defer writer.Close()

	var wg sync.WaitGroup
	errCh := make(chan error, 50)

	// 50 concurrent writes
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			event := &Event{
				Timestamp: time.Now(),
				RequestID: "concurrent-" + string(rune('a'+idx%26)),
				State:     StateQueued,
			}
			if err := writer.Log(event); err != nil {
				errCh <- err
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	// Check for errors
	errCount := 0
	for err := range errCh {
		t.Logf("Concurrent write error: %v", err)
		errCount++
	}

	if errCount > 0 {
		t.Errorf("Expected 0 errors, got %d", errCount)
	}

	// Verify writes
	var count int
	writer.db.QueryRow("SELECT COUNT(*) FROM events").Scan(&count)
	if count != 50 {
		t.Errorf("Expected 50 events, got %d", count)
	}
}

func TestWriter_Close(t *testing.T) {
	tmpDir := t.TempDir()

	writerCacheMu.Lock()
	writerCache = make(map[string]*Writer)
	writerCacheMu.Unlock()

	writer, err := NewWriter(tmpDir, "close-test")
	if err != nil {
		t.Fatalf("NewWriter failed: %v", err)
	}

	err = writer.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestEvent_OptionalFields(t *testing.T) {
	tmpDir := t.TempDir()

	writerCacheMu.Lock()
	writerCache = make(map[string]*Writer)
	writerCacheMu.Unlock()

	writer, err := NewWriter(tmpDir, "optional-test")
	if err != nil {
		t.Fatalf("NewWriter failed: %v", err)
	}
	defer writer.Close()

	// Event with no optional fields
	event := &Event{
		Timestamp: time.Now(),
		RequestID: "minimal-req",
		State:     StateQueued,
		WorkerID:  nil,
		SessionID: nil,
	}

	err = writer.Log(event)
	if err != nil {
		t.Fatalf("Log minimal event failed: %v", err)
	}

	// Verify NULL fields stored correctly
	var workerID, sessionID *string
	err = writer.db.QueryRow(`
		SELECT worker_id, session_id FROM events WHERE request_id = ?
	`, "minimal-req").Scan(&workerID, &sessionID)

	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if workerID != nil {
		t.Error("WorkerID should be nil")
	}
	if sessionID != nil {
		t.Error("SessionID should be nil")
	}
}
