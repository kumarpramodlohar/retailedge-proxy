package cache

import (
	"fmt"
	"time"
)

// Metrics holds a point-in-time snapshot of Near Cache health.
type Metrics struct {
	ProductCount  int
	LastSyncAt    *time.Time
	CacheAgeSecs  float64
	QueuePending  int
	QueueSent     int
	QueueFailed   int
	SchemaVersion string
	QueueCapacity int // maxQueueSize for display
}

// CollectMetrics gathers a snapshot of Near Cache health.
func (db *DB) CollectMetrics() (*Metrics, error) {
	m := &Metrics{
		QueueCapacity: maxQueueSize,
	}
	var err error

	if err = db.conn.QueryRow(
		`SELECT COUNT(*) FROM products`,
	).Scan(&m.ProductCount); err != nil {
		return nil, fmt.Errorf("count products: %w", err)
	}

	var lastSync *string
	if err = db.conn.QueryRow(
		`SELECT MAX(updated_at) FROM products`,
	).Scan(&lastSync); err != nil {
		return nil, fmt.Errorf("last sync: %w", err)
	}

	if lastSync != nil && *lastSync != "" {
		t, parseErr := time.Parse(time.RFC3339, *lastSync)
		if parseErr == nil {
			m.LastSyncAt = &t
			m.CacheAgeSecs = time.Since(t).Seconds()
		}
	}

	m.QueuePending, m.QueueSent, m.QueueFailed, err = db.QueueStats()
	if err != nil {
		return nil, fmt.Errorf("queue stats: %w", err)
	}

	if err = db.conn.QueryRow(
		`SELECT version FROM schema_version ORDER BY id DESC LIMIT 1`,
	).Scan(&m.SchemaVersion); err != nil {
		m.SchemaVersion = "unknown"
	}

	return m, nil
}