package cache

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/mattn/go-sqlite3"
)

// DB wraps the SQLite database connection.
type DB struct {
	conn   *sql.DB
	logger *log.Logger
}

// Open opens the SQLite database, applies PRAGMAs, and returns a ready DB.
// The caller must pass migration files — use cache.LoadMigrations() or
// provide them directly. Returns error if migrations fail or schema mismatches.
func Open(path string, logger *log.Logger, files []MigrationFile) (*DB, error) {
	logger.Printf("opening database at %s", path)

	conn, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Single writer rule: one connection serialises all writes.
	conn.SetMaxOpenConns(1)

	db := &DB{conn: conn, logger: logger}

	if err := db.applyPragmas(); err != nil {
		conn.Close()
		return nil, err
	}

	if err := db.Migrate(files); err != nil {
		conn.Close()
		return nil, err
	}

	return db, nil
}

func (db *DB) applyPragmas() error {
	pragmas := []struct {
		name string
		sql  string
	}{
		{"journal_mode=WAL", "PRAGMA journal_mode=WAL"},
		{"busy_timeout=5000", "PRAGMA busy_timeout=5000"},
		{"foreign_keys=ON",  "PRAGMA foreign_keys=ON"},
	}

	for _, p := range pragmas {
		if _, err := db.conn.Exec(p.sql); err != nil {
			return fmt.Errorf("apply pragma %s: %w", p.name, err)
		}
		db.logger.Printf("pragma set: %s", p.name)
	}

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
func (db *DB) Close() error {
	db.logger.Println("closing database connection")
	return db.conn.Close()
}

// Conn returns the raw *sql.DB for use by other packages.
func (db *DB) Conn() *sql.DB {
	return db.conn
}