package api

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// circuitState represents the three states of the circuit breaker.
type circuiteState int 

const (
	// StateClosed - normal operation. All requests go through.
	StateClosed circuiteState = iota
	// StateOpen - too many failures. Requests fail fast without trying.
	StateOpen
	// StateHalfOpen - cautious recovery. ONe test request allowed.
	StateHalfOpen
)

func (s circuiteState) String() string {
	switch s {
	case StateClosed:
		return "CLOSED"
	case StateOpen:
		return "OPEN"
	case StateHalfOpen:
		return "HALF-OPEN"
	default:
		return "UNKNOWN"
	}
}

const (
	// failureThreshold - consecutive failures before opening circuit.
	failureThreshold = 5
	// resetTimeout - how long to stay OPEN before trying HALF-OPEN.
	resetTimeout = 60 * time.Second
)

// CircuiteBreaker protects the Cloud API from thundering herd on recovery.
// Safe for concurrent use by multiple goroutines.
type CircuitBreaker struct {
	mu sync.Mutex
	state circuiteState
	failures int // consecutive failure count (reset on success)
	lastFailure time.Time // when the most recent failure occured.
	logger *log.Logger
}

// NewCircuitBreaker creates a circuit breaker starting in CLOSED state.
func NewCircuitBreaker (logger *log.Logger) *CircuitBreaker {
	return &CircuitBreaker {
		state: StateClosed,
		logger: logger,
	}
}

// Allow returns nil if the request should proceed, or an error if the
// circuite is OPEN and reset timeout has not expired.
func (cb *CircuitBreaker) Allow() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		return nil
	case StateOpen:
		// Check if enough time has passed to try HALF-OPEN
		if time.Since(cb.lastFailure) >= resetTimeout {
			cb.state = StateHalfOpen
			cb.logger.Printf("circuit breaker: OPEN -> HALF-OPEN (testing recovery)")
			return nil
		}

		remaining := resetTimeout - time.Since(cb.lastFailure)
		return fmt.Errorf("circuit OPEN - Cloud API unavailable, return in  %s", remaining.Round(time.Second))

	case StateHalfOpen:
		// Allow exactly one test request through

		return nil
	}

	return nil
}

// RecordSuccess resets the circuit to CLOSED on a successful send.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state != StateClosed {
		cb.logger.Printf("circuit breaker: %s -> CLOSED (Cloud API recovered)", cb.state)
	}
	cb.state = StateClosed
	cb.failures = 0
}

// RecordFailure increments the failure count and opens the circuit
// if the threashold is reached.

func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures++
	cb.lastFailure = time.Now()

	switch cb.state {
	case StateClosed:
		if cb.failures >= failureThreshold {
			cb.state = StateOpen
			cb.logger.Printf(
				"circuit breaker: CLOSED → OPEN after %d consecutive failures"+
					" — will retry in %s",
				cb.failures, resetTimeout)
		}
	case StateHalfOpen:
		// Test request failed — back to OPEN
		cb.state = StateOpen
		cb.logger.Printf("circuit breaker: HALF-OPEN → OPEN (test request failed)")
	}
}

// State returns the current circuit state for monitoring.
func (cb *CircuitBreaker) State() string {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state.String()
}

// Failures returns the current consecutive failure count.
func (cb *CircuitBreaker) Failures() int {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.failures
}