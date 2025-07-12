package tzkt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// Internal API constants
const (
	defaultLimit     = 100
	delegationsPath  = "/v1/operations/delegations"
	queryParamLimit  = "limit"
	queryParamOffset = "offset"
	queryParamSelect = "select"
	// Select only necessary fields to minimize payload and reduce costs
	defaultSelectFields = "id,timestamp,amount,sender,level"
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

// NewClient creates a new Tzkt API client with explicit dependencies
func NewClient(httpClient *http.Client, baseURL string) *Client {
	return &Client{
		httpClient: httpClient,
		baseURL:    baseURL,
	}
}

// DelegationsRequest represents parameters for getting delegations
type DelegationsRequest struct {
	Limit  uint
	Offset uint
}

// Delegation represents a Tezos delegation from Tzkt API
type Delegation struct {
	ID        int64  `json:"id"`
	Level     int    `json:"level"`
	Timestamp string `json:"timestamp"`
	Sender    struct {
		Address string `json:"address"`
	} `json:"sender"`
	Amount int64 `json:"amount"`
}

// GetDelegations retrieves delegations from the Tzkt API
func (c *Client) GetDelegations(ctx context.Context, req DelegationsRequest) ([]Delegation, error) {
	req.Limit = effectiveLimit(req.Limit)

	httpReq, err := c.buildRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrHTTPRequestFailed, err)
	}
	defer func() {
		// Drain response body to enable connection reuse
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %d", ErrUnexpectedStatus, resp.StatusCode)
	}

	var delegations []Delegation
	if err := json.NewDecoder(resp.Body).Decode(&delegations); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMalformedResponseBody, err)
	}

	return delegations, nil
}

func effectiveLimit(limit uint) uint {
	if limit == 0 {
		return defaultLimit
	}
	return limit
}

func (c *Client) buildRequest(ctx context.Context, req DelegationsRequest) (*http.Request, error) {
	url := c.buildDelegationsURL(req.Limit, req.Offset)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMalformedRequest, err)
	}

	// Add gzip compression support to reduce bandwidth usage and improve performance
	httpReq.Header.Set("Accept-Encoding", "gzip")

	return httpReq, nil
}

func (c *Client) buildDelegationsURL(limit, offset uint) string {
	baseURL := fmt.Sprintf("%s%s?%s=%d&%s=%s",
		c.baseURL, delegationsPath, queryParamLimit, limit, queryParamSelect, defaultSelectFields)

	if offset > 0 {
		return fmt.Sprintf("%s&%s=%d", baseURL, queryParamOffset, offset)
	}
	return baseURL
}
