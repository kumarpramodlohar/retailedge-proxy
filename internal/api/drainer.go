package api

import (
	"context"
	"log"
	"math"
	"math/rand"
	"time"

	"github.com/pramodlohar/retailedge-proxy/internal/cache"
)

const (
	// maxAttempts before a queue entry is abandoned
	maxAttempts = 10

	// pollInterval between drain cycles when queue is empty
	pollInterval = 5 * time.Second

	// batchSize number of entries to drain per cycle
	batchSize = 10

	// baseDelay for exponential backoff
	baseDelay = 1 * time.Second

	// maxDelay caps the backoff ceiling
	maxDelay = 5 * time.Minute
)

// Drainer polls the Change Request Queue and sends pending entries
// to the Cloud REST API with exponential backoff and jitter.
// Runs until ctx is cancelled.
type Drainer struct {
	db     *cache.DB
	client *Client
	logger *log.Logger
}

// NewDrainer creates a Drainer backed by the given DB and API client.
func NewDrainer(db *cache.DB, client *Client, logger *log.Logger) *Drainer {
	return &Drainer{db: db, client: client, logger: logger}
}

// Run starts the drain loop. Blocks until ctx is cancelled.
// On each cycle: fetch pending entries, send each to Cloud API,
// mark sent on success, mark failed on error.
// When queue is empty, sleeps pollInterval before checking again.
func (d *Drainer) Run(ctx context.Context) {
	d.logger.Println("drainer started — polling queue every 5s")

	for {
		select {
		case <-ctx.Done():
			d.logger.Println("drainer stopped")
			return
		default:
			d.drainCycle(ctx)
			select {
			case <-ctx.Done():
				return
			case <-time.After(pollInterval):
			}
		}
	}
}

// drainCycle fetches one batch of pending entries and sends them.
func (d *Drainer) drainCycle(ctx context.Context) {
	entries, err := d.db.PendingChanges(batchSize)
	if err != nil {
		d.logger.Printf("queue read error: %v", err)
		return
	}

	if len(entries) == 0 {
		return // nothing to drain
	}

	d.logger.Printf("draining %d pending change(s)", len(entries))

	for _, entry := range entries {
		if ctx.Err() != nil {
			return // context cancelled mid-batch
		}
		d.send(entry)
	}

	// Log queue stats after each cycle
	pending, sent, failed, err := d.db.QueueStats()
	if err == nil {
		d.logger.Printf("queue stats: pending=%d sent=%d failed=%d",
			pending, sent, failed)
	}
}

// send attempts to POST one entry to the Cloud API.
// Uses exponential backoff with jitter based on attempt count.
func (d *Drainer) send(entry *cache.QueueEntry) {
	// Apply backoff wait based on previous attempts
	if entry.Attempts > 0 {
		delay := backoffDelay(entry.Attempts)
		d.logger.Printf("entry %d: attempt %d — waiting %s before retry",
			entry.ID, entry.Attempts+1, delay.Round(time.Millisecond))
		time.Sleep(delay)
	}

	d.logger.Printf("sending entry %d: product=%s attempt=%d",
		entry.ID, entry.ProductID, entry.Attempts+1)

	err := d.client.SendChange(entry.Payload)
	if err != nil {
		d.logger.Printf("entry %d failed: %v", entry.ID, err)

		if entry.Attempts+1 >= maxAttempts {
			d.logger.Printf("entry %d abandoned after %d attempts", entry.ID, maxAttempts)
			if dbErr := d.db.MarkAbandoned(entry.ID, err.Error()); dbErr != nil {
				d.logger.Printf("mark abandoned error: %v", dbErr)
			}
			return
		}

		if dbErr := d.db.MarkFailed(entry.ID, err.Error()); dbErr != nil {
			d.logger.Printf("mark failed error: %v", dbErr)
		}
		return
	}

	if dbErr := d.db.MarkSent(entry.ID); dbErr != nil {
		d.logger.Printf("mark sent error: %v", dbErr)
		return
	}

	d.logger.Printf("entry %d sent successfully: product=%s", entry.ID, entry.ProductID)
}

// backoffDelay returns exponential backoff with full jitter.
// Formula: random(0, min(maxDelay, baseDelay * 2^attempt))
// Jitter prevents thundering herd when 500 stores reconnect simultaneously.
func backoffDelay(attempt int) time.Duration {
	exp := math.Pow(2, float64(attempt))
	ceiling := time.Duration(float64(baseDelay) * exp)
	if ceiling > maxDelay {
		ceiling = maxDelay
	}
	// Full jitter: random between 0 and ceiling
	jittered := time.Duration(rand.Int63n(int64(ceiling)))
	return jittered
}