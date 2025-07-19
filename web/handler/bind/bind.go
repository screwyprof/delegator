package bind

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/screwyprof/delegator/web/api"
	"github.com/screwyprof/delegator/web/tezos"
)

// Sentinel errors for request binding
var (
	ErrInvalidYear    = errors.New("invalid year parameter")
	ErrInvalidPage    = errors.New("invalid page parameter")
	ErrInvalidPerPage = errors.New("invalid per_page parameter")
)

// GetDelegationsRequest binds HTTP request to DelegationsRequest
func GetDelegationsRequest(r *http.Request) (api.DelegationsRequest, error) {
	query := r.URL.Query()

	year, err := parseUintEmptyAsZero(query.Get("year"))
	if err != nil {
		return api.DelegationsRequest{}, fmt.Errorf("%w: %w", ErrInvalidYear, err)
	}

	page, err := parseUintEmptyAsZero(query.Get("page"))
	if err != nil {
		return api.DelegationsRequest{}, fmt.Errorf("%w: %w", ErrInvalidPage, err)
	}

	perPage, err := parseUintEmptyAsZero(query.Get("per_page"))
	if err != nil {
		return api.DelegationsRequest{}, fmt.Errorf("%w: %w", ErrInvalidPerPage, err)
	}

	return api.DelegationsRequest{
		Year:    year,
		Page:    page,
		PerPage: perPage,
	}, nil
}

// parseUintEmptyAsZero parses string to uint64, treats empty string as 0
func parseUintEmptyAsZero(s string) (uint64, error) {
	if s == "" {
		return 0, nil
	}
	return strconv.ParseUint(s, 10, 64)
}

// GetDelegationsResponse binds domain delegations to API response format
func GetDelegationsResponse(delegations []tezos.Delegation) api.DelegationsResponse {
	apiDelegations := make([]api.Delegation, len(delegations))
	for i, del := range delegations {
		apiDelegations[i] = api.Delegation{
			Timestamp: del.Timestamp.Format(time.RFC3339),
			Amount:    fmt.Sprintf("%d", del.Amount),
			Delegator: del.Delegator,
			Level:     fmt.Sprintf("%d", del.Level),
		}
	}

	return api.DelegationsResponse{
		Data: apiDelegations,
	}
}
