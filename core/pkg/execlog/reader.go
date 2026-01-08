package execlog

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Reader handles querying execution log events from SQLite
type Reader struct {
	db *sql.DB
}

// NewReader creates a new ExecLog reader for the specified agent
func NewReader(execlogDir, agentName string) (*Reader, error) {
	dbPath := filepath.Join(execlogDir, agentName+".db")

	// Open SQLite database in read-only mode
	db, err := sql.Open("sqlite", dbPath+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	return &Reader{db: db}, nil
}

// GetCrashedRequests returns all requests that have been marked as CRASHED
func (r *Reader) GetCrashedRequests() ([]*CrashedRequest, error) {
	rows, err := r.db.Query(`
		SELECT DISTINCT e1.request_id, e1.worker_id, e1.session_id, e2.timestamp as started_at
		FROM events e1
		JOIN events e2 ON e1.request_id = e2.request_id AND e2.state = ?
		WHERE e1.state = ?
		ORDER BY e1.timestamp DESC
	`, StateStarted, StateCrashed)

	if err != nil {
		return nil, fmt.Errorf("query crashed requests: %w", err)
	}
	defer rows.Close()

	var crashed []*CrashedRequest
	for rows.Next() {
		var req CrashedRequest
		var startedAtNano int64
		var workerID, sessionID sql.NullString

		err := rows.Scan(&req.RequestID, &workerID, &sessionID, &startedAtNano)
		if err != nil {
			continue // Skip malformed rows
		}

		req.WorkerID = workerID.String
		if sessionID.Valid {
			req.SessionID = &sessionID.String
		}
		req.StartedAt = time.Unix(0, startedAtNano)

		crashed = append(crashed, &req)
	}

	return crashed, nil
}

// Close closes the database connection
func (r *Reader) Close() error {
	return r.db.Close()
}
