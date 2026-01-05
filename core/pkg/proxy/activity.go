// Package proxy handles agent execution and output capture.
package proxy

import (
	"bufio"
	"io"
	"sync"
	"time"
)

// ActivityMonitor tracks output activity from agent process (Agent-Native timeout)
// It monitors stdout/stderr for activity and triggers timeouts based on:
// - Idle timeout: No activity for N seconds = agent is stuck
// - Max timeout: Total execution time limit (safety cap)
type ActivityMonitor struct {
	mu            sync.Mutex
	lastActivity  time.Time
	startTime     time.Time
	idleTimeout   time.Duration
	maxTimeout    time.Duration
	checkInterval time.Duration
	stopped       bool
	stopChan      chan struct{}
}

// NewActivityMonitor creates a new activity monitor for Agent-Native timeout
func NewActivityMonitor(idleTimeout, maxTimeout, checkInterval time.Duration) *ActivityMonitor {
	return &ActivityMonitor{
		lastActivity:  time.Now(),
		startTime:     time.Now(),
		idleTimeout:   idleTimeout,
		maxTimeout:    maxTimeout,
		checkInterval: checkInterval,
		stopChan:      make(chan struct{}),
	}
}

// MonitorReader wraps a reader and tracks activity
// Each line read resets the activity timer
func (m *ActivityMonitor) MonitorReader(r io.Reader, output io.Writer) io.Reader {
	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()
		scanner := bufio.NewScanner(r)
		// Increase buffer size for large output lines
		const maxCapacity = 1024 * 1024 // 1MB max line size
		buf := make([]byte, 64*1024)
		scanner.Buffer(buf, maxCapacity)

		for scanner.Scan() {
			line := scanner.Bytes()

			// Record activity
			m.mu.Lock()
			m.lastActivity = time.Now()
			m.mu.Unlock()

			// Write to pipe (for caller) and output (for capture)
			pw.Write(line)
			pw.Write([]byte("\n"))
			if output != nil {
				output.Write(line)
				output.Write([]byte("\n"))
			}
		}
	}()

	return pr
}

// CheckTimeout returns true if should timeout, with reason
// Returns: (shouldTimeout, reason)
// reason is one of: "max_timeout", "idle_timeout", ""
func (m *ActivityMonitor) CheckTimeout() (bool, string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()

	// Check max timeout first (absolute limit)
	if m.maxTimeout > 0 && now.Sub(m.startTime) > m.maxTimeout {
		return true, "max_timeout"
	}

	// Check idle timeout (no activity = stuck)
	if m.idleTimeout > 0 && now.Sub(m.lastActivity) > m.idleTimeout {
		return true, "idle_timeout"
	}

	return false, ""
}

// StartWatching starts background timeout checker
// Returns a channel that receives timeout reason when timeout occurs
func (m *ActivityMonitor) StartWatching() <-chan string {
	timeoutChan := make(chan string, 1)

	go func() {
		ticker := time.NewTicker(m.checkInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if shouldTimeout, reason := m.CheckTimeout(); shouldTimeout {
					timeoutChan <- reason
					return
				}
			case <-m.stopChan:
				return
			}
		}
	}()

	return timeoutChan
}

// Stop stops the activity monitor
func (m *ActivityMonitor) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.stopped {
		m.stopped = true
		close(m.stopChan)
	}
}

// GetIdleTime returns how long since last activity
func (m *ActivityMonitor) GetIdleTime() time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	return time.Since(m.lastActivity)
}

// GetElapsedTime returns total elapsed time since start
func (m *ActivityMonitor) GetElapsedTime() time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	return time.Since(m.startTime)
}

// RecordActivity manually records an activity event
// Useful for non-stdout activity (e.g., tool calls, file operations)
func (m *ActivityMonitor) RecordActivity() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastActivity = time.Now()
}
