package execlog

import (
	"database/sql"
	"fmt"
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
		return cached, nil
	}

	// Create new writer
	dbPath := filepath.Join(execlogDir, agentName+".db")

	// Open SQLite database with busy timeout
	db, err := sql.Open("sqlite", dbPath+"?_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
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
