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

	// Specific year validation errors
	ErrYearNotYYYYFormat = errors.New("year must be exactly 4 digits (YYYY format)")
	ErrYearNotNumeric    = errors.New("year must be numeric")
	ErrYearOutOfRange    = errors.New("year must be between 2018 and current year + 10")

	// Specific page validation errors
	ErrPageNotNumeric  = errors.New("page must be numeric")
	ErrPageNotPositive = errors.New("page must be positive")

	// Specific per_page validation errors
	ErrPerPageNotNumeric  = errors.New("per_page must be numeric")
	ErrPerPageNotPositive = errors.New("per_page must be positive")
	ErrPerPageTooLarge    = errors.New("per_page must be between 1 and 100")
)

// GetDelegationsRequest binds HTTP request to DelegationsRequest with defaults
func GetDelegationsRequest(r *http.Request) (api.DelegationsRequest, error) {
	req := api.DelegationsRequest{
		Year:    0,  // 0 means no year filter
		Page:    1,  // Default to first page
		PerPage: 50, // Default pagination size
	}

	query := r.URL.Query()

	// Parse year parameter
	if yearParam := query.Get("year"); yearParam != "" {
		year, err := parseYearYYYY(yearParam)
		if err != nil {
			return req, fmt.Errorf("%w: %w", ErrInvalidYear, err)
		}
		req.Year = year
	}

	// Parse page parameter
	if pageParam := query.Get("page"); pageParam != "" {
		page, err := parsePageNumber(pageParam)
		if err != nil {
			return req, fmt.Errorf("%w: %w", ErrInvalidPage, err)
		}
		req.Page = page
	}

	// Parse per_page parameter
	if perPageParam := query.Get("per_page"); perPageParam != "" {
		perPage, err := parsePerPageLimit(perPageParam)
		if err != nil {
			return req, fmt.Errorf("%w: %w", ErrInvalidPerPage, err)
		}
		req.PerPage = perPage
	}

	return req, nil
}

// parseYearYYYY validates that the year parameter follows YYYY format (4 digits, reasonable range)
// As specified in TASK.md: "year, which is specified in the format YYYY"
func parseYearYYYY(yearParam string) (uint64, error) {
	// Must be exactly 4 characters
	if len(yearParam) != 4 {
		return 0, ErrYearNotYYYYFormat
	}

	// Must parse as a number
	year, err := strconv.ParseUint(yearParam, 10, 64)
	if err != nil {
		return 0, ErrYearNotNumeric
	}

	// Must be in reasonable range for Tezos (launched 2018) + some future buffer
	// Tezos launched in 2018, so years before that don't make sense
	// Upper bound is generous to allow for future data
	currentYear := uint64(time.Now().Year())
	if year < 2018 || year > currentYear+10 {
		return 0, ErrYearOutOfRange
	}

	return year, nil
}

// parsePageNumber validates that the page parameter is a positive integer
func parsePageNumber(pageParam string) (uint64, error) {
	// Must parse as a number
	page, err := strconv.ParseUint(pageParam, 10, 64)
	if err != nil {
		return 0, ErrPageNotNumeric
	}

	// Must be positive
	if page == 0 {
		return 0, ErrPageNotPositive
	}

	return page, nil
}

// parsePerPageLimit validates that the per_page parameter is within acceptable limits
func parsePerPageLimit(perPageParam string) (uint64, error) {
	// Must parse as a number
	perPage, err := strconv.ParseUint(perPageParam, 10, 64)
	if err != nil {
		return 0, ErrPerPageNotNumeric
	}

	// Must be positive and within reasonable limits
	if perPage == 0 {
		return 0, ErrPerPageNotPositive
	}

	if perPage > 100 {
		return 0, ErrPerPageTooLarge
	}

	return perPage, nil
}

// GetDelegationsResponse binds domain delegations to API response format
func GetDelegationsResponse(delegations []tezos.Delegation) api.DelegationsResponse {
	apiDelegations := make([]api.Delegation, len(delegations))
	for i, del := range delegations {
		apiDelegations[i] = api.Delegation{
			Timestamp: del.Timestamp.Format(time.RFC3339),
			Amount:    strconv.FormatInt(del.Amount, 10),
			Delegator: del.Delegator,
			Level:     strconv.FormatInt(del.Level, 10),
		}
	}

	return api.DelegationsResponse{
		Data: apiDelegations,
	}
}
