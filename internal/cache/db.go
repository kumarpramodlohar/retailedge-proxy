package cache

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/mattn/go-sqlite3"
)

// DB wraps the SQLite database connection.
// All three services use this type to access the Near Cache.
type DB struct {
	conn   *sql.DB
	logger *log.Logger
}

// Open opens the SQLite database at path, applies PRAGMAs,
// runs all pending migrations, and returns a ready DB.
// Returns error if any pragma fails, any migration fails,
// or the schema version is ahead of this binary.
func Open(path string, logger *log.Logger) (*DB, error) {
	logger.Printf("opening database at %s", path)

	conn, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Single writer rule: one connection serialises all writes.
	// Readers use WAL snapshots and are never blocked by this.
	conn.SetMaxOpenConns(1)

	db := &DB{conn: conn, logger: logger}

	if err := db.applyPragmas(); err != nil {
		conn.Close()
		return nil, err
	}

	if err := db.Migrate(); err != nil {
		conn.Close()
		return nil, err
	}

	return db, nil
}

// applyPragmas sets required SQLite configuration on every connection.
// Must run before any reads or writes.
func (db *DB) applyPragmas() error {
	pragmas := []struct {
		name string
		sql  string
	}{
		// WAL mode: writers do not block readers.
		{"journal_mode=WAL", "PRAGMA journal_mode=WAL"},
		// busy_timeout: wait up to 5s before returning SQLITE_BUSY.
		{"busy_timeout=5000", "PRAGMA busy_timeout=5000"},
		// foreign_keys: enforce referential integrity.
		{"foreign_keys=ON", "PRAGMA foreign_keys=ON"},
	}

	for _, p := range pragmas {
		if _, err := db.conn.Exec(p.sql); err != nil {
			return fmt.Errorf("apply pragma %s: %w", p.name, err)
		}
		db.logger.Printf("pragma set: %s", p.name)
	}

	// Verify WAL mode was accepted
	var mode string
	if err := db.conn.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		return fmt.Errorf("verify journal_mode: %w", err)
	}
	if mode != "wal" {
		return fmt.Errorf("expected journal_mode=wal, got %s", mode)
	}
	db.logger.Printf("WAL mode confirmed: %s", mode)
	return nil
}

// Close closes the database connection.
// Always defer this after Open succeeds.
func (db *DB) Close() error {
	db.logger.Println("closing database connection")
	return db.conn.Close()
}

// Conn returns the raw *sql.DB for direct queries.
// Used by services to read and write product data.
func (db *DB) Conn() *sql.DB {
	return db.conn
}