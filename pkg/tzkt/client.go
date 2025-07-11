package tzkt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Public configuration constants
const (
	DefaultBaseURL = "https://api.tzkt.io"
	DefaultTimeout = 30 * time.Second
)

// Internal API constants
const (
	delegationsPath = "/v1/operations/delegations"
	queryParamLimit = "limit"
)

// Sentinel errors for different failure modes
var (
	ErrMalformedRequest      = errors.New("malformed request")
	ErrHTTPRequestFailed     = errors.New("http request failed")
	ErrUnexpectedStatus      = errors.New("unexpected HTTP status code")
	ErrMalformedResponseBody = errors.New("malformed response body")
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
			Timeout: DefaultTimeout,
		},
		baseURL: DefaultBaseURL,
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
	url := fmt.Sprintf("%s%s?%s=%d", c.baseURL, delegationsPath, queryParamLimit, req.Limit)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMalformedRequest, err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrHTTPRequestFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %d", ErrUnexpectedStatus, resp.StatusCode)
	}

	var delegations []Delegation
	if err := json.NewDecoder(resp.Body).Decode(&delegations); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMalformedResponseBody, err)
	}

	return delegations, nil
}
