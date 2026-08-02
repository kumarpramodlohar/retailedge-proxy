package cache

import (
	"fmt"
	"time"
)

// Metrics holds a point-in-time snapshot of the Near Cache health.
// Collected by the metrics command and logged by each service.
type Metrics struct {
	// Cache freshness — when was the Near Cache last updated by Events Service
	ProductCount      int
	LastSyncAt        *time.Time
	CacheAgeSecs      float64

	// Queue depth — how many writes are waiting to drain
	QueuePending int
	QueueSent    int
	QueueFailed  int

	// Schema version
	SchemaVersion string
}

// CollectMetrics gathers a snapshot of Near Cache health from the database.
func (db *DB) CollectMetrics() (*Metrics, error) {
	m := &Metrics{}

	// Product count
	if err := db.conn.QueryRow(
		`SELECT COUNT(*) FROM products`,
	).Scan(&m.ProductCount); err != nil {
		return nil, fmt.Errorf("count products: %w", err)
	}

	// Most recent update_at across all products (proxy for last sync time)
	var lastSync *string
	if err := db.conn.QueryRow(
		`SELECT MAX(updated_at) FROM products`,
	).Scan(&lastSync); err != nil {
		return nil, fmt.Errorf("last sync: %w", err)
	}

	if lastSync != nil && *lastSync != "" {
		t, err := time.Parse(time.RFC3339, *lastSync)
		if err == nil {
			m.LastSyncAt = &t
			m.CacheAgeSecs = time.Since(t).Seconds()
		}
	}

	// Queue stats — returns 0 for missing status rows gracefully
	pending, sent, failed, err := db.QueueStats()
	if err != nil {
		return nil, fmt.Errorf("queue stats: %w", err)
	}
	m.QueuePending = pending
	m.QueueSent = sent
	m.QueueFailed = failed

	// Current schema version
	if err := db.conn.QueryRow(
		`SELECT version FROM schema_version ORDER BY id DESC LIMIT 1`,
	).Scan(&m.SchemaVersion); err != nil {
		m.SchemaVersion = "unknown"
	}

	return m, nil
}