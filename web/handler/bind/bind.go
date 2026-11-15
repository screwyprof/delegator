package bind

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/screwyprof/delegator/web/api"
	"github.com/screwyprof/delegator/web/tezos"
)

// Sentinel errors for request binding
var (
	ErrInvalidYear    = errors.New("invalid year parameter")
	ErrInvalidPage    = errors.New("invalid page parameter")
	ErrInvalidPerPage = errors.New("invalid per_page parameter")
	ErrInvalidCursor  = errors.New("invalid cursor")
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
		After:   query.Get("after"),
		Before:  query.Get("before"),
	}, nil
}

// cursorVersion prefixes every token so the format can change without misreading old links
const cursorVersion = "v1"

// EncodeCursor renders a cursor as the opaque, URL-safe token used in after/before links.
// The token records the year filter it was issued under, so it can't be replayed against a
// different one (Google AIP-158, Zalando #137). Microsecond precision matches what Postgres
// stores, so the position round-trips exactly. Clients must treat the token as opaque.
func EncodeCursor(c tezos.Cursor, year uint64) string {
	raw := strings.Join([]string{
		cursorVersion,
		strconv.FormatUint(year, 10),
		strconv.FormatInt(c.Timestamp.UnixMicro(), 10),
		strconv.FormatInt(c.ID, 10),
	}, ":")
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// DecodeCursor parses a token produced by EncodeCursor for a request filtered by year
// (0 = unfiltered). An empty token means no cursor (nil). A token issued under a different
// year filter is rejected rather than silently paging a different result set.
func DecodeCursor(token string, year uint64) (*tezos.Cursor, error) {
	if token == "" {
		return nil, nil
	}

	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidCursor, err)
	}

	parts := strings.Split(string(raw), ":")
	if len(parts) != 4 || parts[0] != cursorVersion {
		return nil, fmt.Errorf("%w: unrecognized format", ErrInvalidCursor)
	}

	tokenYear, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidCursor, err)
	}
	if tokenYear != year {
		return nil, fmt.Errorf("%w: issued for a different year filter", ErrInvalidCursor)
	}

	micros, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidCursor, err)
	}

	// Clients can forge tokens; a time outside years 0001-9999 can't have come from a stored
	// delegation, and extreme values overflow on the way into Postgres, silently wrapping
	// around to an unrelated position instead of failing.
	ts := time.UnixMicro(micros).UTC()
	if ts.Year() < 1 || ts.Year() > 9999 {
		return nil, fmt.Errorf("%w: position out of range", ErrInvalidCursor)
	}

	id, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidCursor, err)
	}

	return &tezos.Cursor{Timestamp: ts, ID: id}, nil
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
