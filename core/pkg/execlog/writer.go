package execlog

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

// Writer handles writing execution log events to SQLite
type Writer struct {
	db *sql.DB
	mu sync.Mutex // Serialize writes to prevent SQLITE_BUSY
}

var (
	writerCacheMu sync.Mutex
	writerCache   = make(map[string]*Writer) // agentName → cached writer
)

// NewWriter creates or returns a cached ExecLog writer for the specified agent
// Writers are cached and reused to avoid connection overhead
func NewWriter(execlogDir, agentName string) (*Writer, error) {
	// Check cache first (thread-safe)
	writerCacheMu.Lock()
	defer writerCacheMu.Unlock()

	if cached, exists := writerCache[agentName]; exists {
		// Validate cached connection is still healthy
		if err := cached.db.Ping(); err != nil {
			log.Printf("[execlog] Cached connection for %s is stale: %v", agentName, err)
			delete(writerCache, agentName)
			cached.db.Close()
		} else {
			return cached, nil
		}
	}

	// Ensure directory exists
	if err := os.MkdirAll(execlogDir, 0755); err != nil {
		return nil, fmt.Errorf("create execlog directory: %w", err)
	}

	// Create new writer
	dbPath := filepath.Join(execlogDir, agentName+".db")

	// Open SQLite database with busy timeout (increased to 10s for better concurrency)
	db, err := sql.Open("sqlite", dbPath+"?_busy_timeout=10000")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Configure connection pool (SQLite = single writer, limit connections)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0) // Reuse connection forever

	// Force connection to actually open (sql.Open is lazy)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	// Create schema
	if err := createSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	// Cache writer (don't close it - reuse connection)
	writer := &Writer{db: db}
	writerCache[agentName] = writer

	return writer, nil
}

// createSchema creates the events table and indexes
func createSchema(db *sql.DB) error {
	// Enable WAL mode for concurrent reads/writes (critical for production)
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return fmt.Errorf("enable WAL: %w", err)
	}

	// Optimize WAL checkpoint behavior (checkpoint every 1000 pages)
	if _, err := db.Exec("PRAGMA wal_autocheckpoint=1000"); err != nil {
		return fmt.Errorf("set wal_autocheckpoint: %w", err)
	}

	schema := `
	CREATE TABLE IF NOT EXISTS events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp INTEGER NOT NULL,
		request_id TEXT NOT NULL,
		state TEXT NOT NULL,
		worker_id TEXT,
		session_id TEXT,
		duration_ms INTEGER,
		error TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_request ON events(request_id);
	CREATE INDEX IF NOT EXISTS idx_state ON events(state);
	CREATE INDEX IF NOT EXISTS idx_timestamp ON events(timestamp);
	`

	if _, err := db.Exec(schema); err != nil {
		log.Printf("[execlog] Base schema creation failed: %v", err)
		return err
	}

	// Safe migration: Add new columns for source tracking (idempotent)
	// ALTER TABLE ADD COLUMN is safe - SQLite ignores if column exists
	migrations := []string{
		`ALTER TABLE events ADD COLUMN source TEXT`,
		`ALTER TABLE events ADD COLUMN mcp_caller TEXT`,
	}

	for _, migration := range migrations {
		_, err := db.Exec(migration)
		// Ignore "duplicate column" error for existing DBs
		if err != nil && !strings.Contains(err.Error(), "duplicate column") {
			log.Printf("[execlog] Migration failed: %s - %v", migration, err)
			return fmt.Errorf("migration failed: %w", err)
		}
	}

	// Index for source filtering
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_source ON events(source)`)

	return nil
}

// Log writes an event to the database (thread-safe)
func (w *Writer) Log(event *Event) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	_, err := w.db.Exec(`
		INSERT INTO events (timestamp, request_id, state, worker_id, session_id, duration_ms, error, source, mcp_caller)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, event.Timestamp.UnixNano(), event.RequestID, event.State,
		event.WorkerID, event.SessionID, event.DurationMs, event.Error,
		event.Source, event.MCPCaller)

	return err
}

// Close closes the database connection
func (w *Writer) Close() error {
	return w.db.Close()
}
