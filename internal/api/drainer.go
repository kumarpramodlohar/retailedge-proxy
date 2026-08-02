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
	maxAttempts  = 10
	pollInterval = 5 * time.Second
	batchSize    = 10
	baseDelay    = 1 * time.Second
	maxDelay     = 5 * time.Minute
)

// Drainer polls the Change Request Queue and drains pending entries
// to the Cloud REST API with exponential backoff, full jitter,
// circuit breaker protection, and per-store rate limiting.
type Drainer struct {
	db      *cache.DB
	client  *Client
	circuit *CircuitBreaker
	logger  *log.Logger
}

// NewDrainer creates a Drainer with an integrated circuit breaker.
func NewDrainer(db *cache.DB, client *Client, logger *log.Logger) *Drainer {
	return &Drainer{
		db:      db,
		client:  client,
		circuit: NewCircuitBreaker(logger),
		logger:  logger,
	}
}

// Run starts the drain loop. Blocks until ctx is cancelled.
func (d *Drainer) Run(ctx context.Context) {
	d.logger.Println("drainer started — polling every 5s with circuit breaker")

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

// drainCycle fetches one batch and attempts to send each entry.
func (d *Drainer) drainCycle(ctx context.Context) {
	// Check circuit breaker before even fetching from queue
	if err := d.circuit.Allow(); err != nil {
		d.logger.Printf("circuit breaker blocking drain: %v", err)
		return
	}

	entries, err := d.db.PendingChanges(batchSize)
	if err != nil {
		d.logger.Printf("queue read error: %v", err)
		return
	}

	if len(entries) == 0 {
		return
	}

	d.logger.Printf("draining %d pending change(s) [circuit=%s]",
		len(entries), d.circuit.State())

	for _, entry := range entries {
		if ctx.Err() != nil {
			return
		}
		d.send(entry)
	}

	// Log queue stats after each cycle
	pending, sent, failed, err := d.db.QueueStats()
	if err == nil {
		d.logger.Printf("queue stats: pending=%d sent=%d failed=%d circuit=%s",
			pending, sent, failed, d.circuit.State())
	}
}

// send attempts one entry with circuit breaker + backoff + rate limiter.
func (d *Drainer) send(entry *cache.QueueEntry) {
	// Apply backoff wait based on previous attempts
	if entry.Attempts > 0 {
		delay := backoffDelay(entry.Attempts)
		d.logger.Printf("entry %d: attempt %d — backoff %s [circuit=%s]",
			entry.ID, entry.Attempts+1,
			delay.Round(time.Millisecond),
			d.circuit.State())
		time.Sleep(delay)
	}

	// Check circuit breaker before this specific send
	if err := d.circuit.Allow(); err != nil {
		d.logger.Printf("entry %d: circuit breaker blocked — %v", entry.ID, err)
		// Do not increment attempts — the entry is not being tried, just deferred
		return
	}

	d.logger.Printf("sending entry %d: product=%s attempt=%d",
		entry.ID, entry.ProductID, entry.Attempts+1)

	// Rate limiter is inside client.SendChange()
	err := d.client.SendChange(entry.Payload)
	if err != nil {
		d.circuit.RecordFailure()
		d.logger.Printf("entry %d failed [failures=%d circuit=%s]: %v",
			entry.ID, d.circuit.Failures(), d.circuit.State(), err)

		if entry.Attempts+1 >= maxAttempts {
			d.logger.Printf("entry %d abandoned after %d attempts",
				entry.ID, maxAttempts)
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

	// Success — record in circuit breaker and mark entry sent
	d.circuit.RecordSuccess()

	if dbErr := d.db.MarkSent(entry.ID); dbErr != nil {
		d.logger.Printf("mark sent error: %v", dbErr)
		return
	}

	d.logger.Printf("entry %d sent successfully: product=%s [circuit=%s]",
		entry.ID, entry.ProductID, d.circuit.State())
}

// backoffDelay returns exponential backoff with full jitter.
// Formula: random(0, min(maxDelay, baseDelay * 2^attempt))
func backoffDelay(attempt int) time.Duration {
	exp := math.Pow(2, float64(attempt))
	ceiling := time.Duration(float64(baseDelay) * exp)
	if ceiling > maxDelay {
		ceiling = maxDelay
	}
	return time.Duration(rand.Int63n(int64(ceiling)))
}

// CircuitState returns the current circuit breaker state for monitoring.
func (d *Drainer) CircuitState() string {
	return d.circuit.State()
}