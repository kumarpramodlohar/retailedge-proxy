package cache

import (
	"database/sql"
	"fmt"
	"time"
)

// Product mirrors the products table row exactly.
// Field order matches the SELECT column order in every query.
type Product struct {
	ID        string
	Name      string
	Price     float64
	Category  string
	InStock   bool
	Version   int
	UpdatedAt time.Time
}

// GetProduct returns one product by ID.
// Returns sql.ErrNoRows if the product does not exist —
// the caller maps this to gRPC NOT_FOUND status.
func (db *DB) GetProduct(id string) (*Product, error) {
	row := db.conn.QueryRow(`
		SELECT id, name, price, category, in_stock, version, updated_at
		FROM products
		WHERE id = ?`, id)

	return scanProduct(row)
}

// ListProducts returns all products, optionally filtered by category.
// Returns an empty slice (not an error) when no products match.
func (db *DB) ListProducts(category string) ([]*Product, error) {
	var rows *sql.Rows
	var err error

	if category == "" {
		rows, err = db.conn.Query(`
			SELECT id, name, price, category, in_stock, version, updated_at
			FROM products
			ORDER BY name`)
	} else {
		rows, err = db.conn.Query(`
			SELECT id, name, price, category, in_stock, version, updated_at
			FROM products
			WHERE category = ?
			ORDER BY name`, category)
	}
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	defer rows.Close()

	var products []*Product
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			return nil, fmt.Errorf("scan product row: %w", err)
		}
		products = append(products, p)
	}
	return products, rows.Err()
}

// UpsertProduct inserts or updates a product in the Near Cache.
// Called by the Events Service when an inbound MDM change arrives.
// This is the ONLY function that writes to the products table.
func (db *DB) UpsertProduct(p *Product) error {
	inStock := 0
	if p.InStock {
		inStock = 1
	}

	_, err := db.conn.Exec(`
		INSERT INTO products (id, name, price, category, in_stock, version, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name       = excluded.name,
			price      = excluded.price,
			category   = excluded.category,
			in_stock   = excluded.in_stock,
			version    = excluded.version,
			updated_at = excluded.updated_at`,
		p.ID,
		p.Name,
		p.Price,
		p.Category,
		inStock,
		p.Version,
		p.UpdatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("upsert product %s: %w", p.ID, err)
	}
	return nil
}

// scanProduct reads one product row from either *sql.Row or *sql.Rows.
// Both implement the same Scan interface so this works for both.
type scanner interface {
	Scan(dest ...any) error
}

func scanProduct(s scanner) (*Product, error) {
	var p Product
	var updatedAt string
	var inStock int // SQLite stores booleans as integers

	err := s.Scan(
		&p.ID,
		&p.Name,
		&p.Price,
		&p.Category,
		&inStock,
		&p.Version,
		&updatedAt,
	)
	if err != nil {
		return nil, err
	}

	p.InStock = inStock != 0

	p.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse updated_at %q: %w", updatedAt, err)
	}

	return &p, nil
}