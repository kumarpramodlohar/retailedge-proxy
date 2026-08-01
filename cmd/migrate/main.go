package main

import (
	"log"
	"os"

	"github.com/pramodlohar/retailedge-proxy/internal/cache"
)

func main() {
	logger := log.New(os.Stdout, "[migrate] ", log.LstdFlags)
	logger.Println("RetailEdge migration runner starting")

	// In P2 this path comes from site config.
	// For P1 verification we use a fixed test path.
	dbPath := "/tmp/retailedge-test.db"

	db, err := cache.Open(dbPath, logger)
	if err != nil {
		logger.Fatalf("FATAL: %v", err)
	}
	defer db.Close()

	logger.Println("migration runner complete — database is ready")

	// Verify tables
	rows, err := db.Conn().Query(
		`SELECT name FROM sqlite_master WHERE type='table' ORDER BY name`,
	)
	if err != nil {
		logger.Fatalf("FATAL: list tables: %v", err)
	}
	defer rows.Close()

	logger.Println("tables in database:")
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			logger.Fatalf("FATAL: %v", err)
		}
		logger.Printf("  → %s", name)
	}

	// Verify WAL mode
	var mode string
	if err := db.Conn().QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		logger.Fatalf("FATAL: check journal_mode: %v", err)
	}
	logger.Printf("journal_mode: %s", mode)

	// Show schema_version contents
	vRows, err := db.Conn().Query(
		`SELECT version, applied_at, description FROM schema_version ORDER BY version`,
	)
	if err != nil {
		logger.Fatalf("FATAL: query schema_version: %v", err)
	}
	defer vRows.Close()

	logger.Println("schema_version table:")
	for vRows.Next() {
		var version, appliedAt, description string
		if err := vRows.Scan(&version, &appliedAt, &description); err != nil {
			logger.Fatalf("FATAL: %v", err)
		}
		logger.Printf("  → %s | %s | %s", version, appliedAt, description)
	}
}