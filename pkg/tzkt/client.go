package tzkt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Internal API constants
const (
	defaultLimit     = 100
	delegationsPath  = "/v1/operations/delegations"
	queryParamLimit  = "limit"
	queryParamSelect = "select"
	// Select only necessary fields to minimize payload
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

// DelegationsRequest represents parameters for getting delegations with filtering
type DelegationsRequest struct {
	Limit         uint
	Offset        uint       // offset pagination
	IDGreaterThan *int64     // id.gt filter
	TimestampGE   *time.Time // timestamp.ge filter
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

// GetDelegations retrieves delegations from the Tzkt API with filtering support
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
	fullURL := c.buildDelegationsURL(req)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMalformedRequest, err)
	}

	return httpReq, nil
}

func (c *Client) buildDelegationsURL(req DelegationsRequest) string {
	params := url.Values{}
	params.Set(queryParamLimit, strconv.FormatUint(uint64(req.Limit), 10))
	params.Set(queryParamSelect, defaultSelectFields)

	// Add filtering parameters
	if req.IDGreaterThan != nil {
		params.Set("id.gt", strconv.FormatInt(*req.IDGreaterThan, 10))
	}
	if req.TimestampGE != nil {
		params.Set("timestamp.ge", req.TimestampGE.Format(time.RFC3339))
	}

	// Add offset pagination if specified
	if req.Offset > 0 {
		params.Set("offset", strconv.FormatUint(uint64(req.Offset), 10))
	}

	return fmt.Sprintf("%s%s?%s", c.baseURL, delegationsPath, params.Encode())
}
