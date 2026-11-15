// Package api defines the web API's wire types: request parameters, response bodies and
// HTTP-aware errors.
package api

// DelegationsRequest represents the query parameters for GET /xtz/delegations.
type DelegationsRequest struct {
	Year    uint64 `query:"year"`     // Optional year filter in YYYY format
	Page    uint64 `query:"page"`     // Page number for pagination (default: 1); not combinable with a cursor
	PerPage uint64 `query:"per_page"` // Number of items per page (default: 50, max: 100)
	After   string `query:"after"`    // Opaque cursor from a rel="next" link: the page after it
	Before  string `query:"before"`   // Opaque cursor from a rel="prev" link: the page before it
}

// Delegation represents a single delegation in the API response.
type Delegation struct {
	Timestamp string `json:"timestamp"`
	Amount    string `json:"amount"`
	Delegator string `json:"delegator"`
	Level     string `json:"level"`
}

// DelegationsResponse represents the API response format for GET /xtz/delegations.
type DelegationsResponse struct {
	Data []Delegation `json:"data"`
}
