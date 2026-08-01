package events

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/pramodlohar/retailedge-proxy/internal/cache"
)

// ProductEvent is the JSON payload published to the Pub/Sub topic
// by the cloud MDM when a product is created or updated.
// event_id is used for de-duplication — processing the same event
// twice must produce the same result (idempotent by design).
type ProductEvent struct {
	ProductID string  `json:"product_id"`
	Name      string  `json:"name"`
	Price     float64 `json:"price"`
	Category  string  `json:"category"`
	InStock   bool    `json:"in_stock"`
	Version   int     `json:"version"`
	EventID   string  `json:"event_id"`
	EventType string  `json:"event_type"`
}

// Handler processes inbound Pub/Sub messages and writes to the Near Cache.
// It is the only component that writes to the products table — the single
// writer rule is enforced here by design.
type Handler struct {
	db *cache.DB
}

// NewHandler creates a Handler backed by the given database.
func NewHandler(db *cache.DB) *Handler {
	return &Handler{db: db}
}

// Handle parses one raw Pub/Sub message payload and upserts the product
// into the Near Cache. Returns an error if parsing or writing fails —
// the subscriber will not ack the message and Pub/Sub will redeliver it.
func (h *Handler) Handle(data []byte) error {
	var event ProductEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return fmt.Errorf("parse event JSON: %w", err)
	}

	if event.ProductID == "" {
		return fmt.Errorf("event missing product_id")
	}
	if event.EventID == "" {
		return fmt.Errorf("event missing event_id")
	}

	// Convert to cache.Product and upsert.
	// ON CONFLICT(id) DO UPDATE in UpsertProduct makes this idempotent —
	// receiving the same event twice writes the same data twice, no harm.
	p := &cache.Product{
		ID:        event.ProductID,
		Name:      event.Name,
		Price:     event.Price,
		Category:  event.Category,
		InStock:   event.InStock,
		Version:   event.Version,
		UpdatedAt: time.Now().UTC(),
	}

	if err := h.db.UpsertProduct(p); err != nil {
		return fmt.Errorf("upsert product %s: %w", event.ProductID, err)
	}

	return nil
}