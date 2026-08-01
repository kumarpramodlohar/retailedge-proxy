package cache

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"
)

// migrationFiles embeds all SQL files from the sql/ subdirectory.
// The cache package owns its own schema — no external package needed.
//
//go:embed sql/*.sql
var migrationFiles embed.FS

// migration is the internal parsed form of a SQL migration file.
type migration struct {
	version     string
	description string
	sql         string
}

// MigrationFile is no longer needed by callers — kept for compatibility
// with cmd/migrate during transition. Will be removed in P2.

// Migrate runs all pending migrations in version order.
// Safe to call on every startup — skips already-applied migrations.
// Refuses to start if DB schema is ahead of known migrations.
func (db *DB) Migrate() error {
	db.logger.Println("starting migration runner")

	available, err := db.loadMigrations()
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}
	db.logger.Printf("found %d migration(s)", len(available))

	if err := db.bootstrapVersionTable(available); err != nil {
		return fmt.Errorf("bootstrap version table: %w", err)
	}

	applied, err := db.appliedVersions()
	if err != nil {
		return fmt.Errorf("read applied versions: %w", err)
	}

	if err := db.checkVersionMismatch(available, applied); err != nil {
		return err
	}

	pending := db.pendingMigrations(available, applied)
	if len(pending) == 0 {
		db.logger.Println("schema is up to date — no migrations to run")
		return nil
	}

	db.logger.Printf("%d pending migration(s) to apply", len(pending))
	for _, m := range pending {
		if err := db.applyMigration(m); err != nil {
			return fmt.Errorf("apply migration %s: %w", m.version, err)
		}
	}

	db.logger.Println("all migrations applied successfully")
	return nil
}

// loadMigrations reads all .sql files from the embedded sql/ directory,
// parses their version numbers, and returns them sorted by version.
func (db *DB) loadMigrations() ([]migration, error) {
	var result []migration

	err := fs.WalkDir(migrationFiles, "sql", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".sql") {
			return nil
		}

		content, err := migrationFiles.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}

		// Version is the numeric prefix before the first underscore
		// e.g. "001_create_products.sql" → version "001"
		parts := strings.SplitN(d.Name(), "_", 2)
		if len(parts) < 2 {
			return fmt.Errorf("filename %s must match NNN_description.sql", d.Name())
		}

		desc := strings.TrimSuffix(parts[1], ".sql")
		desc = strings.ReplaceAll(desc, "_", " ")

		result = append(result, migration{
			version:     parts[0],
			description: desc,
			sql:         string(content),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].version < result[j].version
	})

	return result, nil
}

func (db *DB) bootstrapVersionTable(available []migration) error {
	for _, m := range available {
		if m.version == "001" {
			if _, err := db.conn.Exec(m.sql); err != nil {
				return fmt.Errorf("bootstrap schema_version: %w", err)
			}
			var count int
			err := db.conn.QueryRow(
				`SELECT COUNT(*) FROM schema_version WHERE version = ?`,
				m.version,
			).Scan(&count)
			if err != nil {
				return fmt.Errorf("check if 001 recorded: %w", err)
			}
			if count == 0 {
				_, err = db.conn.Exec(
					`INSERT INTO schema_version (version, applied_at, description)
					 VALUES (?, ?, ?)`,
					m.version,
					time.Now().UTC().Format(time.RFC3339),
					m.description,
				)
				if err != nil {
					return fmt.Errorf("record migration 001: %w", err)
				}
				db.logger.Printf("migration %s bootstrapped: %s", m.version, m.description)
			}
			return nil
		}
	}
	return fmt.Errorf("migration 001 not found — cannot bootstrap")
}

func (db *DB) appliedVersions() (map[string]bool, error) {
	rows, err := db.conn.Query(
		`SELECT version FROM schema_version ORDER BY version`,
	)
	if err != nil {
		return nil, fmt.Errorf("query schema_version: %w", err)
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("scan version: %w", err)
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

func (db *DB) checkVersionMismatch(available []migration, applied map[string]bool) error {
	known := make(map[string]bool)
	for _, m := range available {
		known[m.version] = true
	}
	for v := range applied {
		if !known[v] {
			return fmt.Errorf(
				"FATAL: database has migration %s but binary does not — "+
					"schema is ahead of this binary, deploy correct version first", v)
		}
	}
	return nil
}

func (db *DB) pendingMigrations(available []migration, applied map[string]bool) []migration {
	var pending []migration
	for _, m := range available {
		if !applied[m.version] {
			pending = append(pending, m)
		}
	}
	return pending
}

func (db *DB) applyMigration(m migration) error {
	db.logger.Printf("applying migration %s: %s", m.version, m.description)

	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(m.sql); err != nil {
		return fmt.Errorf("execute SQL: %w", err)
	}

	_, err = tx.Exec(
		`INSERT INTO schema_version (version, applied_at, description)
		 VALUES (?, ?, ?)`,
		m.version,
		time.Now().UTC().Format(time.RFC3339),
		m.description,
	)
	if err != nil {
		return fmt.Errorf("record in schema_version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	db.logger.Printf("migration %s applied successfully", m.version)
	return nil
}