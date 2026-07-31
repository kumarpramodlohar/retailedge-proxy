-- Migration 002: create products Near Cache table
-- This is the primary table for Product master data.
-- All reads from the gRPC Listener come from this table.
-- The Events Service is the only writer (single writer rule).

CREATE TABLE IF NOT EXISTS products (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    price DECIMAL(10, 2) NOT NULL,
    category TEXT NOT NULL,
    in_stock INTEGER NOT NULL DEFAULT 1,
    version INTEGER NOT NULL DEFAULT 1,
    updated_at TEXT NOT NULL
);
-- Index for fast category lookups (common read pattern)
CREATE INDEX IF NOT EXISTS idx_products_category ON products (category);
-- Index for fast freshness checks (used by monitoring)
CREATE INDEX IF NOT EXISTS idx_products_updated_at ON products (updated_at);
