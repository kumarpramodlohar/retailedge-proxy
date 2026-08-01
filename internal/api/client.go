package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client posts product changes to the Cloud REST API.
// In production this is the MDM system's inbound API.
// In P3/P5 this is the mock Cloud Run service.
type Client struct {
	baseURL    string
	httpClient *http.Client
	storeID    string
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
// Returns nil on HTTP 200 or 202 (accepted).
// Returns error on any other status or network failure.
func (c *Client) SendChange(payload string) error {
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