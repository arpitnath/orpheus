package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // Registers as "sqlite" driver
)

// Store manages API keys in SQLite database.
type Store struct {
	db *sql.DB
}

// APIKey represents an API key with metadata.
type APIKey struct {
	Key               string
	Name              string
	OrgID             string     // Organization identifier (for multi-tenancy)
	CreatedAt         time.Time
	ExpiresAt         *time.Time
	RequestsPerMinute int
	IsActive          bool
}

// NewStore creates or opens the API key database.
func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath) // modernc.org/sqlite registers as "sqlite"
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Create tables if they don't exist
	if err := createTables(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("create tables: %w", err)
	}

	return &Store{db: db}, nil
}

// createTables creates the database schema.
func createTables(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS api_keys (
		key TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		org_id TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		expires_at TIMESTAMP,
		requests_per_minute INTEGER DEFAULT 100,
		is_active BOOLEAN DEFAULT 1
	);

	CREATE TABLE IF NOT EXISTS request_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		api_key TEXT NOT NULL,
		timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		endpoint TEXT NOT NULL,
		status_code INTEGER
	);

	CREATE INDEX IF NOT EXISTS idx_request_log_key_time
	ON request_log(api_key, timestamp);
	`

	if _, err := db.Exec(schema); err != nil {
		return err
	}

	// Migrate existing keys: add org_id if missing
	migrateSchema := `
	UPDATE api_keys
	SET org_id = 'org-' || substr(hex(randomblob(8)), 1, 12)
	WHERE org_id IS NULL OR org_id = '';
	`

	_, err := db.Exec(migrateSchema)
	return err
}

// CreateKey generates a new API key.
func (s *Store) CreateKey(name string, requestsPerMinute int) (*APIKey, error) {
	// Generate random key
	key := generateKey()

	// Derive org_id from API key hash (consistent with deploy_handler)
	orgID := deriveOrgID(key)

	// Insert into database
	query := `
		INSERT INTO api_keys (key, name, org_id, requests_per_minute)
		VALUES (?, ?, ?, ?)
	`

	_, err := s.db.Exec(query, key, name, orgID, requestsPerMinute)
	if err != nil {
		return nil, fmt.Errorf("insert key: %w", err)
	}

	// Return created key
	return s.GetKey(key)
}

// deriveOrgID derives org_id from API key using SHA256 hash.
// This matches the derivation in deploy_handler.go.
func deriveOrgID(apiKey string) string {
	hash := sha256.Sum256([]byte(apiKey))
	return "org-" + hex.EncodeToString(hash[:])[:12]
}

// ValidateKey checks if a key is valid and active.
func (s *Store) ValidateKey(key string) (*APIKey, error) {
	apiKey, err := s.GetKey(key)
	if err != nil {
		return nil, err
	}

	if !apiKey.IsActive {
		return nil, fmt.Errorf("key is inactive")
	}

	// Check expiration
	if apiKey.ExpiresAt != nil && apiKey.ExpiresAt.Before(time.Now()) {
		return nil, fmt.Errorf("key has expired")
	}

	return apiKey, nil
}

// GetKey retrieves a key by its value.
func (s *Store) GetKey(key string) (*APIKey, error) {
	query := `
		SELECT key, name, org_id, created_at, expires_at, requests_per_minute, is_active
		FROM api_keys
		WHERE key = ?
	`

	var apiKey APIKey
	var expiresAt sql.NullTime
	var orgID sql.NullString

	err := s.db.QueryRow(query, key).Scan(
		&apiKey.Key,
		&apiKey.Name,
		&orgID,
		&apiKey.CreatedAt,
		&expiresAt,
		&apiKey.RequestsPerMinute,
		&apiKey.IsActive,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("key not found")
	}
	if err != nil {
		return nil, fmt.Errorf("query key: %w", err)
	}

	if orgID.Valid {
		apiKey.OrgID = orgID.String
	}

	if expiresAt.Valid {
		apiKey.ExpiresAt = &expiresAt.Time
	}

	return &apiKey, nil
}

// ListKeys returns all API keys.
func (s *Store) ListKeys() ([]*APIKey, error) {
	query := `
		SELECT key, name, org_id, created_at, expires_at, requests_per_minute, is_active
		FROM api_keys
		ORDER BY created_at DESC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query keys: %w", err)
	}
	defer rows.Close()

	var keys []*APIKey
	for rows.Next() {
		var apiKey APIKey
		var expiresAt sql.NullTime
		var orgID sql.NullString

		err := rows.Scan(
			&apiKey.Key,
			&apiKey.Name,
			&orgID,
			&apiKey.CreatedAt,
			&expiresAt,
			&apiKey.RequestsPerMinute,
			&apiKey.IsActive,
		)
		if err != nil {
			return nil, fmt.Errorf("scan key: %w", err)
		}

		if orgID.Valid {
			apiKey.OrgID = orgID.String
		}

		if expiresAt.Valid {
			apiKey.ExpiresAt = &expiresAt.Time
		}

		keys = append(keys, &apiKey)
	}

	return keys, nil
}

// RevokeKey deactivates an API key.
func (s *Store) RevokeKey(key string) error {
	query := `UPDATE api_keys SET is_active = 0 WHERE key = ?`

	result, err := s.db.Exec(query, key)
	if err != nil {
		return fmt.Errorf("revoke key: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("key not found")
	}

	return nil
}

// LogRequest logs a request for analytics/debugging.
func (s *Store) LogRequest(apiKey, endpoint string, statusCode int) error {
	query := `
		INSERT INTO request_log (api_key, endpoint, status_code)
		VALUES (?, ?, ?)
	`

	_, err := s.db.Exec(query, apiKey, endpoint, statusCode)
	if err != nil {
		// Don't fail request if logging fails (best-effort)
		return err
	}

	return nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// generateKey generates a secure random API key.
func generateKey() string {
	// Generate 32 bytes of random data
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}

	// Encode as base64 URL-safe
	encoded := base64.URLEncoding.EncodeToString(b)

	// Add prefix
	return "agsk_" + encoded
}
