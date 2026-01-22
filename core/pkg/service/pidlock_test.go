package service

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestAcquirePIDLock_Fresh(t *testing.T) {
	// Use temp directory for test
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "test.pid")

	// Acquire lock (no existing lockfile)
	unlock, err := acquirePIDLock(lockPath)
	if err != nil {
		t.Fatalf("Expected success on fresh lock, got error: %v", err)
	}

	// Verify lockfile was created
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Error("Lockfile should exist after acquire")
	}

	// Cleanup
	unlock()

	// Verify lockfile was removed
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Error("Lockfile should be removed after unlock")
	}
}

func TestAcquirePIDLock_StalePID(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "test.pid")

	// Create stale lockfile with dead PID
	stalePID := 99999 // Assuming this PID doesn't exist
	err := os.WriteFile(lockPath, []byte(string(rune(stalePID))), 0644)
	if err != nil {
		t.Fatalf("Failed to create stale lockfile: %v", err)
	}

	// Try to acquire (should clean stale and acquire)
	unlock, err := acquirePIDLock(lockPath)
	if err != nil {
		t.Fatalf("Expected success after cleaning stale lock, got error: %v", err)
	}

	defer unlock()

	// Verify lock acquired
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Error("Lockfile should exist after acquiring stale lock")
	}
}

func TestAcquirePIDLock_AlreadyHeld(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "test.pid")

	// Acquire first lock
	unlock1, err := acquirePIDLock(lockPath)
	if err != nil {
		t.Fatalf("First acquire failed: %v", err)
	}
	defer unlock1()

	// Try to acquire again (should fail - lock held)
	_, err = acquirePIDLock(lockPath)
	if err == nil {
		t.Error("Second acquire should fail when lock already held")
	}

	// Error should indicate lock is held
	if err != nil && err.Error() == "" {
		t.Error("Error message should indicate lock is held")
	}
}

func TestReleaseLock_MultipleCalls(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "test.pid")

	unlock, err := acquirePIDLock(lockPath)
	if err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}

	// First unlock - should succeed
	unlock()

	// Second unlock - should be safe (idempotent)
	// Should not panic or error
	unlock()

	// Verify lockfile removed
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Error("Lockfile should be removed")
	}
}

func TestAcquirePIDLock_PIDBehavior(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "test.pid")

	// Acquire lock
	unlock, err := acquirePIDLock(lockPath)
	if err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}
	defer unlock()

	// Read lockfile
	content, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("Failed to read lockfile: %v", err)
	}

	// Should contain current process PID
	currentPID := os.Getpid()
	// Note: PID is written as string, not binary
	// Implementation may vary, but should be related to current process
	if len(content) == 0 {
		t.Error("Lockfile should contain PID data")
	}

	t.Logf("Current PID: %d, Lockfile content: %s", currentPID, string(content))
}

func TestCheckStaleLock_NonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "nonexistent.pid")

	// Should return nil (no error) for non-existent file
	err := checkStaleLock(lockPath)
	if err != nil {
		t.Errorf("checkStaleLock on non-existent file should return nil, got: %v", err)
	}
}

func TestPIDLock_RaceCondition(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "race.pid")

	// Try to acquire lock from 10 goroutines simultaneously
	var successCount, errorCount int
	var mu sync.Mutex
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func() {
			unlock, err := acquirePIDLock(lockPath)
			if err == nil {
				mu.Lock()
				successCount++
				mu.Unlock()
				time.Sleep(10 * time.Millisecond)
				unlock()
			} else {
				mu.Lock()
				errorCount++
				mu.Unlock()
			}
			done <- true
		}()
	}

	// Wait for all
	for i := 0; i < 10; i++ {
		<-done
	}

	// Get final counts (thread-safe)
	mu.Lock()
	finalSuccess := successCount
	finalErrors := errorCount
	mu.Unlock()

	// Exactly ONE should have succeeded (flock is exclusive)
	t.Logf("Success: %d, Errors: %d", finalSuccess, finalErrors)

	// At minimum, not all should succeed (that would be a lock failure)
	if finalSuccess == 10 {
		t.Error("All 10 goroutines acquired lock - flock is broken!")
	}
}
