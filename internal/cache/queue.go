package cache

import (
	"fmt"
	"time"
)

// QueueEntry represents one pending outbound write in the Change Request Queue.
type QueueEntry struct {
	ID          int64
	ProductID   string
	Payload     string // JSON payload to POST to Cloud API
	Status      string // pending | sending | sent | failed
	Attempts    int
	CreatedAt   time.Time
	LastAttempt *time.Time
	Error       string
}

const maxQueueSize = 10000

// EnqueueChange adds a product change to the outbound queue.
// Returns an error if the queue is full — protects disk from unbounded growth.
// At 1 write/second a full queue represents ~2.7 hours of offline operation.
func (db *DB) EnqueueChange(productID string, payload string) error {
	// Check queue depth before inserting
	var pending int
	if err := db.conn.QueryRow(
		`SELECT COUNT(*) FROM change_request_queue WHERE status = 'pending'`,
	).Scan(&pending); err != nil {
		return fmt.Errorf("check queue size: %w", err)
	}

	if pending >= maxQueueSize {
		return fmt.Errorf(
			"queue full (%d/%d pending) — store may be offline too long: "+
				"writes rejected until queue drains",
			pending, maxQueueSize)
	}

	_, err := db.conn.Exec(`
		INSERT INTO change_request_queue
			(product_id, payload, status, attempts, created_at)
		VALUES (?, ?, 'pending', 0, ?)`,
		productID,
		payload,
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("enqueue change for product %s: %w", productID, err)
	}
	return nil
}

// PendingChanges returns up to limit entries with status=pending,
// ordered by created_at (FIFO). Called by the API Service drainer.
func (db *DB) PendingChanges(limit int) ([]*QueueEntry, error) {
	rows, err := db.conn.Query(`
		SELECT id, product_id, payload, status, attempts, created_at,
		       last_attempt, error
		FROM change_request_queue
		WHERE status = 'pending'
		ORDER BY created_at ASC
		LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("query pending changes: %w", err)
	}
	defer rows.Close()

	var entries []*QueueEntry
	for rows.Next() {
		e, err := scanQueueEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("scan queue entry: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// MarkSent marks a queue entry as successfully sent to the Cloud API.
// The entry stays in the table as an audit record.
func (db *DB) MarkSent(id int64) error {
	_, err := db.conn.Exec(`
		UPDATE change_request_queue
		SET status = 'sent', last_attempt = ?
		WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339),
		id,
	)
	if err != nil {
		return fmt.Errorf("mark sent %d: %w", id, err)
	}
	return nil
}

// MarkFailed increments the attempt counter and records the error.
// Status stays 'pending' so the drainer will retry.
// After maxAttempts the caller should set status='failed'.
func (db *DB) MarkFailed(id int64, errMsg string) error {
	_, err := db.conn.Exec(`
		UPDATE change_request_queue
		SET attempts = attempts + 1,
		    last_attempt = ?,
		    error = ?
		WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339),
		errMsg,
		id,
	)
	if err != nil {
		return fmt.Errorf("mark failed %d: %w", id, err)
	}
	return nil
}

// MarkAbandoned sets status='failed' after too many retries.
// The entry is kept for inspection but will not be retried.
func (db *DB) MarkAbandoned(id int64, errMsg string) error {
	_, err := db.conn.Exec(`
		UPDATE change_request_queue
		SET status = 'failed',
		    attempts = attempts + 1,
		    last_attempt = ?,
		    error = ?
		WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339),
		errMsg,
		id,
	)
	if err != nil {
		return fmt.Errorf("mark abandoned %d: %w", id, err)
	}
	return nil
}

// QueueStats returns counts by status for monitoring.
func (db *DB) QueueStats() (pending, sent, failed int, err error) {
	rows, err := db.conn.Query(`
		SELECT status, COUNT(*) FROM change_request_queue GROUP BY status`)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("queue stats: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return 0, 0, 0, err
		}
		switch status {
		case "pending":
			pending = count
		case "sent":
			sent = count
		case "failed":
			failed = count
		}
	}
	return pending, sent, failed, rows.Err()
}

type queueScanner interface {
	Scan(dest ...any) error
}

func scanQueueEntry(s queueScanner) (*QueueEntry, error) {
	var e QueueEntry
	var createdAt string
	var lastAttempt *string
	var errStr *string

	err := s.Scan(
		&e.ID,
		&e.ProductID,
		&e.Payload,
		&e.Status,
		&e.Attempts,
		&createdAt,
		&lastAttempt,
		&errStr,
	)
	if err != nil {
		return nil, err
	}

	e.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}

	if lastAttempt != nil {
		t, err := time.Parse(time.RFC3339, *lastAttempt)
		if err != nil {
			return nil, fmt.Errorf("parse last_attempt: %w", err)
		}
		e.LastAttempt = &t
	}

	if errStr != nil {
		e.Error = *errStr
	}

	return &e, nil
}