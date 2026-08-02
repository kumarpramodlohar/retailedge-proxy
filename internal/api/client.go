package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

const (
	// minSendInterval caps the send rate to 10 requests/second per store.
	// 500 stores × 10/sec = 5,000/sec maximum to Cloud API on recovery.
	minSendInterval = 100 * time.Millisecond
)

// Client posts product changes to the Cloud REST API.
// Includes a rate limiter to prevent thundering herd on WAN recovery.
type Client struct {
	baseURL     string
	httpClient  *http.Client
	storeID     string
	mu          sync.Mutex
	lastSendAt  time.Time // rate limiter state
}

// NewClient creates an API client for the given Cloud API base URL.
func NewClient(baseURL string, storeID string) *Client {
	return &Client{
		baseURL: baseURL,
		storeID: storeID,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SendChange POSTs a product change payload to the Cloud REST API.
// Applies rate limiting before sending — enforces minSendInterval between calls.
// Returns nil on HTTP 200 or 202. Returns error on any other status or failure.
func (c *Client) SendChange(payload string) error {
	// Rate limiter — enforce minimum gap between sends
	c.mu.Lock()
	elapsed := time.Since(c.lastSendAt)
	if elapsed < minSendInterval {
		wait := minSendInterval - elapsed
		c.mu.Unlock()
		time.Sleep(wait)
		c.mu.Lock()
	}
	c.lastSendAt = time.Now()
	c.mu.Unlock()

	// Inject store_id into the payload
	var data map[string]any
	if err := json.Unmarshal([]byte(payload), &data); err != nil {
		return fmt.Errorf("parse payload: %w", err)
	}
	data["store_id"] = c.storeID

	body, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	url := c.baseURL + "/v1/products/changes"
	resp, err := c.httpClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("POST %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("cloud API returned %d", resp.StatusCode)
	}

	return nil
}