-- Migration 003: create change request queue table
-- This is the local write queue for outbound product changes.
-- The gRPC Listener enqueues writes here when the Java client submits changes.
-- The API Service drains this table to the Cloud REST API.
-- Entries stay here until the cloud confirms receipt (status=sent).
-- If the WAN is down, entries accumulate and drain when connectivity returns.

CREATE TABLE IF NOT EXISTS change_request_queue (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    product_id   TEXT    NOT NULL,
    payload      TEXT    NOT NULL,
    status       TEXT    NOT NULL DEFAULT 'pending',
    attempts     INTEGER NOT NULL DEFAULT 0,
    created_at   TEXT    NOT NULL,
    last_attempt TEXT,
    error        TEXT
);

-- Index for fast pending queue reads
CREATE INDEX IF NOT EXISTS idx_queue_status
    ON change_request_queue(status);

-- Index for ordering by creation time (drain in FIFO order)
CREATE INDEX IF NOT EXISTS idx_queue_created_at
    ON change_request_queue(created_at);