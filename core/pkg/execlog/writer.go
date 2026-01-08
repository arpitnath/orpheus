package execlog

import (
	"database/sql"
	"fmt"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Writer handles writing execution log events to SQLite
type Writer struct {
	db *sql.DB
}

// NewWriter creates a new ExecLog writer for the specified agent
func NewWriter(execlogDir, agentName string) (*Writer, error) {
	dbPath := filepath.Join(execlogDir, agentName+".db")

	// Open SQLite database
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Create schema if not exists
	if err := createSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	return &Writer{db: db}, nil
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

	_, err := db.Exec(schema)
	return err
}

// Log writes an event to the database
func (w *Writer) Log(event *Event) error {
	_, err := w.db.Exec(`
		INSERT INTO events (timestamp, request_id, state, worker_id, session_id, duration_ms, error)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, event.Timestamp.UnixNano(), event.RequestID, event.State,
		event.WorkerID, event.SessionID, event.DurationMs, event.Error)

	return err
}

// Close closes the database connection
func (w *Writer) Close() error {
	return w.db.Close()
}
