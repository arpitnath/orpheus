package execlog

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Retention manages automatic cleanup of old execution log entries
type Retention struct {
	enabled         bool
	retentionDays   int
	cleanupInterval time.Duration
	batchSize       int
	execlogDir      string

	// Lifecycle management
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	running bool
	mu      sync.RWMutex
}

// NewRetention creates a new retention manager
func NewRetention(execlogDir string, retentionDays int, cleanupInterval time.Duration) *Retention {
	return &Retention{
		enabled:         true,
		retentionDays:   retentionDays,
		cleanupInterval: cleanupInterval,
		batchSize:       10000, // Delete 10k rows at a time
		execlogDir:      execlogDir,
	}
}

// Start begins the retention cleanup loop
func (r *Retention) Start(ctx context.Context) error {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return nil // Already running
	}

	r.ctx, r.cancel = context.WithCancel(ctx)
	r.running = true
	r.mu.Unlock()

	r.wg.Add(1)
	go r.cleanupLoop()

	log.Printf("[execlog-retention] Started (retention=%dd, interval=%v)", r.retentionDays, r.cleanupInterval)
	return nil
}

// cleanupLoop runs periodic cleanup
func (r *Retention) cleanupLoop() {
	defer r.wg.Done()

	ticker := time.NewTicker(r.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.ctx.Done():
			log.Printf("[execlog-retention] Cleanup loop stopped")
			return
		case <-ticker.C:
			r.performCleanup()
		}
	}
}

// performCleanup deletes old entries from all agent databases
func (r *Retention) performCleanup() {
	cutoff := time.Now().AddDate(0, 0, -r.retentionDays)
	cutoffNano := cutoff.UnixNano()

	log.Printf("[execlog-retention] Running cleanup (deleting entries older than %v)", cutoff.Format(time.RFC3339))

	// Find all agent databases
	pattern := filepath.Join(r.execlogDir, "*.db")
	files, err := filepath.Glob(pattern)
	if err != nil {
		log.Printf("[execlog-retention] Error globbing databases: %v", err)
		return
	}

	totalCleaned := 0
	for _, dbFile := range files {
		// Extract agent name from filename
		base := filepath.Base(dbFile)
		agentName := strings.TrimSuffix(base, ".db")

		// Skip SQLite internal files
		if strings.HasSuffix(agentName, "-wal") || strings.HasSuffix(agentName, "-shm") {
			continue
		}

		if err := r.cleanupAgent(agentName, cutoffNano); err != nil {
			log.Printf("[execlog-retention] Error cleaning %s: %v", agentName, err)
		} else {
			totalCleaned++
		}
	}

	log.Printf("[execlog-retention] Cleanup complete (%d agents processed)", totalCleaned)
}

// cleanupAgent deletes old entries from a single agent's database
func (r *Retention) cleanupAgent(agentName string, cutoffNano int64) error {
	dbPath := filepath.Join(r.execlogDir, agentName+".db")

	// Open DB (separate connection from cached Writer)
	db, err := sql.Open("sqlite", dbPath+"?_busy_timeout=5000")
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	totalDeleted := 0

	// Delete in batches to prevent long locks
	for {
		result, err := db.Exec(`
			DELETE FROM events
			WHERE timestamp < ?
			LIMIT ?
		`, cutoffNano, r.batchSize)

		if err != nil {
			return fmt.Errorf("delete batch: %w", err)
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("get rows affected: %w", err)
		}

		if rowsAffected == 0 {
			break // No more rows to delete
		}

		totalDeleted += int(rowsAffected)

		// Small delay between batches to avoid excessive locking
		select {
		case <-r.ctx.Done():
			return r.ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}

	if totalDeleted > 0 {
		log.Printf("[execlog-retention] Deleted %d entries from %s", totalDeleted, agentName)

		// Reclaim disk space with VACUUM
		if _, err := db.Exec("VACUUM"); err != nil {
			log.Printf("[execlog-retention] Warning: VACUUM failed for %s: %v", agentName, err)
		}
	}

	return nil
}

// Stop halts the cleanup loop
func (r *Retention) Stop() error {
	r.mu.Lock()
	if !r.running {
		r.mu.Unlock()
		return nil
	}
	r.running = false
	r.mu.Unlock()

	if r.cancel != nil {
		r.cancel()
	}

	r.wg.Wait() // Wait for cleanup loop to finish

	log.Printf("[execlog-retention] Stopped")
	return nil
}
