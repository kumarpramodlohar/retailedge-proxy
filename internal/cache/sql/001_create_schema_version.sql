-- Migration 001: create schema version tracking table
-- This table records every migration that has been applied.
-- The migration runner reads this table to know where to resume.
-- Forward-only: never delete rows from this table.

CREATE TABLE IF NOT EXISTS schema_version (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    version     TEXT    NOT NULL UNIQUE,
    applied_at  TEXT    NOT NULL,
    description TEXT    NOT NULL
);