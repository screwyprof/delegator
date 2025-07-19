package api

// DelegationsRequest represents the query parameters for GET /xtz/delegations
type DelegationsRequest struct {
	Year    uint64 `query:"year"`     // Optional year filter in YYYY format
	Page    uint64 `query:"page"`     // Page number for pagination (default: 1)
	PerPage uint64 `query:"per_page"` // Number of items per page (default: 50, max: 100)
}

// Delegation represents a single delegation in the API response
type Delegation struct {
	Timestamp string `json:"timestamp"`
	Amount    string `json:"amount"`
	Delegator string `json:"delegator"`
	Level     string `json:"level"`
}

// DelegationsResponse represents the API response format for GET /xtz/delegations
type DelegationsResponse struct {
	Data []Delegation `json:"data"`
}
