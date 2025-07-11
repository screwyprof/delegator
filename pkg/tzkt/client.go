package tzkt

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client represents a Tzkt API client
type Client struct {
	httpClient *http.Client
	baseURL    string
}

// NewClient creates a new Tzkt API client
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL: "https://api.tzkt.io",
	}
}

// NewClientWithHTTP creates a new Tzkt API client with custom HTTP client and base URL
func NewClientWithHTTP(httpClient *http.Client, baseURL string) *Client {
	return &Client{
		httpClient: httpClient,
		baseURL:    baseURL,
	}
}

// DelegationsRequest represents parameters for getting delegations
type DelegationsRequest struct {
	Limit  int
	Offset int
}

// Delegation represents a Tezos delegation from Tzkt API
type Delegation struct {
	Level     int    `json:"level"`
	Timestamp string `json:"timestamp"`
	Sender    struct {
		Address string `json:"address"`
	} `json:"sender"`
	Amount int64 `json:"amount"`
}

// GetDelegations retrieves delegations from the Tzkt API
func (c *Client) GetDelegations(ctx context.Context, req DelegationsRequest) ([]Delegation, error) {
	url := fmt.Sprintf("%s/v1/operations/delegations?limit=%d", c.baseURL, req.Limit)

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var delegations []Delegation
	if err := json.NewDecoder(resp.Body).Decode(&delegations); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return delegations, nil
}
