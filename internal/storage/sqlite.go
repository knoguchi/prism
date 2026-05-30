package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DB wraps a SQLite database connection
type DB struct {
	conn *sql.DB
}

// New creates a new SQLite database connection
func New(dbPath string) (*DB, error) {
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Open database with SQLite-specific pragmas for performance
	dsn := fmt.Sprintf("%s?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)", dbPath)

	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool settings
	conn.SetMaxOpenConns(1) // SQLite only supports one writer
	conn.SetMaxIdleConns(1)

	db := &DB{conn: conn}

	// Run migrations
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return db, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.conn.Close()
}

// Conn returns the underlying sql.DB for custom queries
func (db *DB) Conn() *sql.DB {
	return db.conn
}

// migrate runs all database migrations
func (db *DB) migrate() error {
	migrations := []string{
		migrationCreateRequests,
		migrationCreateResponses,
		migrationCreateWebSocketMessages,
		migrationCreateSchemas,
		migrationCreateEndpointPatterns,
		migrationCreateConfig,
		migrationCreateIndexes,
		migrationCreateFTS,
		migrationCreateSchemaVersions,
	}

	for i, migration := range migrations {
		if _, err := db.conn.ExecContext(context.Background(), migration); err != nil {
			return fmt.Errorf("migration %d failed: %w", i+1, err)
		}
	}

	return nil
}

const migrationCreateRequests = `
CREATE TABLE IF NOT EXISTS requests (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT UNIQUE NOT NULL,
    method TEXT NOT NULL,
    url TEXT NOT NULL,
    host TEXT NOT NULL,
    path TEXT NOT NULL,
    query_string TEXT,
    headers TEXT NOT NULL,
    body BLOB,
    body_size INTEGER DEFAULT 0,
    content_type TEXT,
    protocol TEXT DEFAULT 'HTTP/1.1',
    is_https BOOLEAN DEFAULT 0,
    remote_addr TEXT,
    captured_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    tags TEXT,
    notes TEXT
);
`

const migrationCreateResponses = `
CREATE TABLE IF NOT EXISTS responses (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    request_id INTEGER NOT NULL,
    uuid TEXT UNIQUE NOT NULL,
    status_code INTEGER NOT NULL,
    status_text TEXT,
    headers TEXT NOT NULL,
    body BLOB,
    body_size INTEGER DEFAULT 0,
    content_type TEXT,
    latency_ms INTEGER,
    captured_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (request_id) REFERENCES requests(id) ON DELETE CASCADE
);
`

const migrationCreateWebSocketMessages = `
CREATE TABLE IF NOT EXISTS websocket_messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    request_id INTEGER NOT NULL,
    uuid TEXT UNIQUE NOT NULL,
    direction TEXT NOT NULL,
    message_type TEXT NOT NULL,
    payload BLOB,
    payload_size INTEGER DEFAULT 0,
    sequence_num INTEGER NOT NULL,
    captured_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (request_id) REFERENCES requests(id) ON DELETE CASCADE
);
`

const migrationCreateSchemas = `
CREATE TABLE IF NOT EXISTS schemas (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    host TEXT NOT NULL,
    method TEXT NOT NULL,
    path_pattern TEXT NOT NULL,
    format TEXT NOT NULL,
    content TEXT NOT NULL,
    sample_count INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(host, method, path_pattern, format)
);
`

const migrationCreateEndpointPatterns = `
CREATE TABLE IF NOT EXISTS endpoint_patterns (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    host TEXT NOT NULL,
    method TEXT NOT NULL,
    path_pattern TEXT NOT NULL,
    path_regex TEXT NOT NULL,
    request_schema TEXT,
    response_schemas TEXT,
    query_params TEXT,
    auth_type TEXT,
    sample_count INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(host, method, path_pattern)
);
`

const migrationCreateConfig = `
CREATE TABLE IF NOT EXISTS config (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
`

const migrationCreateIndexes = `
CREATE INDEX IF NOT EXISTS idx_requests_host ON requests(host);
CREATE INDEX IF NOT EXISTS idx_requests_path ON requests(path);
CREATE INDEX IF NOT EXISTS idx_requests_method ON requests(method);
CREATE INDEX IF NOT EXISTS idx_requests_captured_at ON requests(captured_at DESC);
CREATE INDEX IF NOT EXISTS idx_requests_content_type ON requests(content_type);
CREATE INDEX IF NOT EXISTS idx_responses_request_id ON responses(request_id);
CREATE INDEX IF NOT EXISTS idx_responses_status_code ON responses(status_code);
CREATE INDEX IF NOT EXISTS idx_websocket_request_id ON websocket_messages(request_id);
CREATE INDEX IF NOT EXISTS idx_websocket_direction ON websocket_messages(direction);
CREATE INDEX IF NOT EXISTS idx_endpoint_patterns_host ON endpoint_patterns(host);
CREATE INDEX IF NOT EXISTS idx_schemas_host ON schemas(host);
`

const migrationCreateFTS = `
CREATE VIRTUAL TABLE IF NOT EXISTS requests_fts USING fts5(
    uuid,
    url,
    headers,
    body,
    content=requests,
    content_rowid=id
);
`

const migrationCreateSchemaVersions = `
CREATE TABLE IF NOT EXISTS schema_versions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    host TEXT NOT NULL,
    format TEXT NOT NULL,
    version INTEGER NOT NULL,
    content TEXT NOT NULL,
    change_reason TEXT,
    validation_errors_fixed TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    is_active BOOLEAN DEFAULT 0,
    UNIQUE(host, format, version)
);
CREATE INDEX IF NOT EXISTS idx_schema_versions_host ON schema_versions(host, format);
CREATE INDEX IF NOT EXISTS idx_schema_versions_active ON schema_versions(host, format, is_active);
`
