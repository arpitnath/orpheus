package runtime

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestStreamEvent(t *testing.T) {
	event := StreamEvent{
		Type:      "test",
		Timestamp: time.Now(),
		Data:      map[string]interface{}{"key": "value"},
	}

	if event.Type != "test" {
		t.Errorf("Expected Type=test, got %s", event.Type)
	}

	if event.Data["key"] != "value" {
		t.Error("Data not preserved")
	}
}

func TestNewChunkEvent(t *testing.T) {
	event := NewChunkEvent("stdout", "hello world")

	if event.Type != "chunk" {
		t.Errorf("Expected Type=chunk, got %s", event.Type)
	}

	if event.Data["stream"] != "stdout" {
		t.Errorf("Expected stream=stdout, got %v", event.Data["stream"])
	}

	if event.Data["content"] != "hello world" {
		t.Errorf("Expected content='hello world', got %v", event.Data["content"])
	}

	if event.Timestamp.IsZero() {
		t.Error("Timestamp should be set")
	}
}

func TestNewChunkEvent_Stderr(t *testing.T) {
	event := NewChunkEvent("stderr", "error message")

	if event.Data["stream"] != "stderr" {
		t.Errorf("Expected stream=stderr, got %v", event.Data["stream"])
	}

	if event.Data["content"] != "error message" {
		t.Errorf("Expected content='error message', got %v", event.Data["content"])
	}
}

func TestNewProgressEvent(t *testing.T) {
	event := NewProgressEvent(1500, 256)

	if event.Type != "progress" {
		t.Errorf("Expected Type=progress, got %s", event.Type)
	}

	if event.Data["elapsed_ms"] != int64(1500) {
		t.Errorf("Expected elapsed_ms=1500, got %v", event.Data["elapsed_ms"])
	}

	if event.Data["memory_mb"] != 256 {
		t.Errorf("Expected memory_mb=256, got %v", event.Data["memory_mb"])
	}
}

func TestNewCompletedEvent(t *testing.T) {
	output := map[string]interface{}{"result": "success"}
	event := NewCompletedEvent("success", 5000, output)

	if event.Type != "completed" {
		t.Errorf("Expected Type=completed, got %s", event.Type)
	}

	if event.Data["status"] != "success" {
		t.Errorf("Expected status=success, got %v", event.Data["status"])
	}

	if event.Data["duration_ms"] != int64(5000) {
		t.Errorf("Expected duration_ms=5000, got %v", event.Data["duration_ms"])
	}

	if event.Data["output"] == nil {
		t.Error("Output should be set")
	}
}

func TestNewErrorEvent(t *testing.T) {
	event := NewErrorEvent("execution failed", 1)

	if event.Type != "error" {
		t.Errorf("Expected Type=error, got %s", event.Type)
	}

	if event.Data["error"] != "execution failed" {
		t.Errorf("Expected error='execution failed', got %v", event.Data["error"])
	}

	if event.Data["exit_code"] != 1 {
		t.Errorf("Expected exit_code=1, got %v", event.Data["exit_code"])
	}
}

// mockResponseWriter implements http.ResponseWriter with Flusher
type mockResponseWriter struct {
	bytes.Buffer
	headers http.Header
	flushed bool
}

func newMockResponseWriter() *mockResponseWriter {
	return &mockResponseWriter{
		headers: make(http.Header),
	}
}

func (m *mockResponseWriter) Header() http.Header {
	return m.headers
}

func (m *mockResponseWriter) WriteHeader(code int) {
	// No-op for tests
}

func (m *mockResponseWriter) Flush() {
	m.flushed = true
}

func TestNewSSEWriter(t *testing.T) {
	w := newMockResponseWriter()

	sseWriter := NewSSEWriter(w)
	if sseWriter == nil {
		t.Fatal("NewSSEWriter returned nil for valid Flusher")
	}
}

func TestNewSSEWriter_NoFlusher(t *testing.T) {
	// Create a minimal ResponseWriter without Flusher
	rec := httptest.NewRecorder()

	// httptest.ResponseRecorder implements Flusher, so this should work
	sseWriter := NewSSEWriter(rec)
	if sseWriter == nil {
		t.Error("NewSSEWriter should work with httptest.ResponseRecorder")
	}
}

func TestSSEWriter_WriteEvent(t *testing.T) {
	w := newMockResponseWriter()
	sseWriter := NewSSEWriter(w)

	event := NewChunkEvent("stdout", "test output")

	err := sseWriter.WriteEvent(event)
	if err != nil {
		t.Fatalf("WriteEvent failed: %v", err)
	}

	// Verify SSE format written
	output := w.String()
	if !bytes.Contains([]byte(output), []byte("event: chunk\n")) {
		t.Error("Output should contain 'event: chunk'")
	}

	if !bytes.Contains([]byte(output), []byte("data: ")) {
		t.Error("Output should contain 'data: '")
	}

	// Verify flushed
	if !w.flushed {
		t.Error("WriteEvent should flush")
	}
}

func TestSSEWriter_Close(t *testing.T) {
	w := newMockResponseWriter()
	sseWriter := NewSSEWriter(w)

	err := sseWriter.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Write after close should fail
	event := NewChunkEvent("stdout", "test")
	err = sseWriter.WriteEvent(event)
	if err == nil {
		t.Error("WriteEvent after Close should fail")
	}
}

func TestSSEWriter_Concurrent(t *testing.T) {
	w := newMockResponseWriter()
	sseWriter := NewSSEWriter(w)

	var wg sync.WaitGroup
	errCh := make(chan error, 50)

	// 50 concurrent writes
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			event := NewChunkEvent("stdout", "concurrent")
			if err := sseWriter.WriteEvent(event); err != nil {
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
}

func TestRuncResult(t *testing.T) {
	result := RuncResult{
		Stdout:   "hello",
		Stderr:   "error output",
		Err:      nil,
		ExitCode: 0,
		OOMKill:  false,
	}

	if result.Stdout != "hello" {
		t.Error("Stdout mismatch")
	}

	if result.Stderr != "error output" {
		t.Error("Stderr mismatch")
	}

	if result.ExitCode != 0 {
		t.Error("ExitCode mismatch")
	}

	if result.OOMKill {
		t.Error("OOMKill should be false")
	}
}

func TestRuncResult_OOMKill(t *testing.T) {
	result := RuncResult{
		ExitCode: 137,
		OOMKill:  true,
	}

	if !result.OOMKill {
		t.Error("OOMKill should be true for exit 137")
	}

	if result.ExitCode != 137 {
		t.Error("ExitCode should be 137")
	}
}
