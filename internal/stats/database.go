package stats

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"

	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

// Database handles SQLite persistence for statistics and request logs.
type Database struct {
	db     *sql.DB
	mu     sync.RWMutex
	logger *slog.Logger
}

// CountEntry represents a label and count pair in sorted order.
type CountEntry struct {
	Label string
	Count int
}

// Schema for the database tables
const schema = `
-- Stats table (replaces stats.json)
CREATE TABLE IF NOT EXISTS stats (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    start_time TIMESTAMP NOT NULL,
    total_requests INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- IP tracking (replaces IPCounts map)
CREATE TABLE IF NOT EXISTS ip_counts (
    ip TEXT PRIMARY KEY CHECK(length(ip) <= 45 AND length(ip) > 0),
    count INTEGER NOT NULL DEFAULT 1 CHECK(count > 0),
    first_seen TIMESTAMP NOT NULL,
    last_seen TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_ip_count ON ip_counts(count DESC);

-- User agent tracking (replaces UserAgents map)
CREATE TABLE IF NOT EXISTS user_agent_counts (
    user_agent TEXT PRIMARY KEY CHECK(length(user_agent) <= 512 AND length(user_agent) > 0),
    count INTEGER NOT NULL DEFAULT 1 CHECK(count > 0),
    first_seen TIMESTAMP NOT NULL,
    last_seen TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_ua_count ON user_agent_counts(count DESC);

-- Request log (replaces requests.ndjson)
CREATE TABLE IF NOT EXISTS request_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    ip TEXT NOT NULL CHECK(length(ip) <= 45 AND length(ip) > 0),
    user_agent TEXT CHECK(user_agent IS NULL OR length(user_agent) <= 512),
    path TEXT CHECK(path IS NULL OR length(path) <= 2048),
    timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_request_timestamp ON request_log(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_request_ip ON request_log(ip);
`

// NewDatabase creates a new database connection and initializes the schema.
//
// Parameters:
//   - dbPath: path to the SQLite database file
//   - logger: structured logger instance
//
// Returns a new Database instance or an error if initialization fails.
func NewDatabase(dbPath string, logger *slog.Logger) (*Database, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(1) // SQLite works best with single writer
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	// Configure SQLite for security and performance
	// Enable WAL mode for better concurrency and crash recovery
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	// Enable foreign key constraints for data integrity
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	// Set synchronous mode for balance between safety and performance
	// NORMAL provides good durability with better performance than FULL
	if _, err := db.Exec("PRAGMA synchronous=NORMAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to set synchronous mode: %w", err)
	}

	// Execute schema
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}

	// Initialize stats row if it doesn't exist
	_, err = db.Exec(`
		INSERT OR IGNORE INTO stats (id, start_time, total_requests)
		VALUES (1, ?, 0)
	`, time.Now())
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize stats: %w", err)
	}

	logger.Debug("Database initialized", "path", dbPath)

	return &Database{
		db:     db,
		logger: logger,
	}, nil
}

// RecordRequest records a request in the database.
//
// This updates the request log, IP counts, user agent counts, and total request count.
// Uses a transaction to ensure atomic updates.
//
// Parameters:
//   - ctx: context for cancellation and timeout control
//   - req: the request information to record
//
// Returns an error if the database operation fails or context is cancelled.
func (d *Database) RecordRequest(ctx context.Context, req RequestInfo) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert request log entry
	_, err = tx.ExecContext(ctx, `
		INSERT INTO request_log (ip, user_agent, path, timestamp)
		VALUES (?, ?, ?, ?)
	`, req.IP, req.UserAgent, req.Path, req.Timestamp)
	if err != nil {
		return fmt.Errorf("failed to insert request log: %w", err)
	}

	// Update IP count
	_, err = tx.ExecContext(ctx, `
		INSERT INTO ip_counts (ip, count, first_seen, last_seen)
		VALUES (?, 1, ?, ?)
		ON CONFLICT(ip) DO UPDATE SET
			count = count + 1,
			last_seen = ?
	`, req.IP, req.Timestamp, req.Timestamp, req.Timestamp)
	if err != nil {
		return fmt.Errorf("failed to update IP count: %w", err)
	}

	// Update user agent count
	_, err = tx.ExecContext(ctx, `
		INSERT INTO user_agent_counts (user_agent, count, first_seen, last_seen)
		VALUES (?, 1, ?, ?)
		ON CONFLICT(user_agent) DO UPDATE SET
			count = count + 1,
			last_seen = ?
	`, req.UserAgent, req.Timestamp, req.Timestamp, req.Timestamp)
	if err != nil {
		return fmt.Errorf("failed to update user agent count: %w", err)
	}

	// Increment total requests
	_, err = tx.ExecContext(ctx, `
		UPDATE stats
		SET total_requests = total_requests + 1,
		    updated_at = ?
		WHERE id = 1
	`, time.Now())
	if err != nil {
		return fmt.Errorf("failed to update total requests: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetStats retrieves the current statistics.
//
// Parameters:
//   - ctx: context for cancellation and timeout control
//
// Returns the Stats instance populated from the database.
func (d *Database) GetStats(ctx context.Context) (*Stats, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	stats := &Stats{
		IPCounts:   make(map[string]int),
		UserAgents: make(map[string]int),
	}

	// Get start time and total requests
	err := d.db.QueryRowContext(ctx, `
		SELECT start_time, total_requests
		FROM stats
		WHERE id = 1
	`).Scan(&stats.StartTime, &stats.TotalRequests)
	if err != nil {
		return nil, fmt.Errorf("failed to query stats: %w", err)
	}

	// Get IP counts
	rows, err := d.db.QueryContext(ctx, `SELECT ip, count FROM ip_counts`)
	if err != nil {
		return nil, fmt.Errorf("failed to query IP counts: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var ip string
		var count int
		if err := rows.Scan(&ip, &count); err != nil {
			return nil, fmt.Errorf("failed to scan IP count: %w", err)
		}
		stats.IPCounts[ip] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating IP counts: %w", err)
	}

	// Get user agent counts
	rows, err = d.db.QueryContext(ctx, `SELECT user_agent, count FROM user_agent_counts`)
	if err != nil {
		return nil, fmt.Errorf("failed to query user agent counts: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var ua string
		var count int
		if err := rows.Scan(&ua, &count); err != nil {
			return nil, fmt.Errorf("failed to scan user agent count: %w", err)
		}
		stats.UserAgents[ua] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating user agent counts: %w", err)
	}

	// Get recent requests
	recentRequests, err := d.GetRecentRequests(ctx, 100)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent requests: %w", err)
	}
	stats.RecentRequests = recentRequests
	stats.MaxRecentRequests = 100

	return stats, nil
}

// GetRecentRequests retrieves the most recent N requests from the database.
//
// Parameters:
//   - ctx: context for cancellation and timeout control
//   - limit: maximum number of requests to retrieve
//
// Returns a slice of RequestInfo ordered by timestamp (most recent first).
func (d *Database) GetRecentRequests(ctx context.Context, limit int) ([]RequestInfo, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	rows, err := d.db.QueryContext(ctx, `
		SELECT ip, user_agent, path, timestamp
		FROM request_log
		ORDER BY timestamp DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query recent requests: %w", err)
	}
	defer rows.Close()

	var requests []RequestInfo
	for rows.Next() {
		var req RequestInfo
		if err := rows.Scan(&req.IP, &req.UserAgent, &req.Path, &req.Timestamp); err != nil {
			return nil, fmt.Errorf("failed to scan request: %w", err)
		}
		requests = append(requests, req)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating requests: %w", err)
	}

	return requests, nil
}

// GetTopIPs retrieves the top N IP addresses by request count.
//
// Parameters:
//   - ctx: context for cancellation and timeout control
//   - limit: maximum number of IPs to retrieve
//
// Returns a slice of CountEntry ordered by count (highest first).
func (d *Database) GetTopIPs(ctx context.Context, limit int) ([]CountEntry, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	rows, err := d.db.QueryContext(ctx, `
		SELECT ip, count
		FROM ip_counts
		ORDER BY count DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query top IPs: %w", err)
	}
	defer rows.Close()

	var result []CountEntry
	for rows.Next() {
		var entry CountEntry
		if err := rows.Scan(&entry.Label, &entry.Count); err != nil {
			return nil, fmt.Errorf("failed to scan IP: %w", err)
		}
		result = append(result, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating IPs: %w", err)
	}

	return result, nil
}

// GetTopUserAgents retrieves the top N user agents by request count.
//
// Parameters:
//   - ctx: context for cancellation and timeout control
//   - limit: maximum number of user agents to retrieve
//
// Returns a slice of CountEntry ordered by count (highest first).
func (d *Database) GetTopUserAgents(ctx context.Context, limit int) ([]CountEntry, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	rows, err := d.db.QueryContext(ctx, `
		SELECT user_agent, count
		FROM user_agent_counts
		ORDER BY count DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query top user agents: %w", err)
	}
	defer rows.Close()

	var result []CountEntry
	for rows.Next() {
		var entry CountEntry
		if err := rows.Scan(&entry.Label, &entry.Count); err != nil {
			return nil, fmt.Errorf("failed to scan user agent: %w", err)
		}
		result = append(result, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating user agents: %w", err)
	}

	return result, nil
}

// Close closes the database connection.
//
// Returns an error if the close operation fails.
func (d *Database) Close() error {
	if d.db != nil {
		return d.db.Close()
	}
	return nil
}
