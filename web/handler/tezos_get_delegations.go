package handler

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/screwyprof/delegator/pkg/httpkit"
	"github.com/screwyprof/delegator/web/api"
	"github.com/screwyprof/delegator/web/handler/bind"
	"github.com/screwyprof/delegator/web/tezos"
)

const GetDelegationsRoute = http.MethodGet + " " + "/xtz/delegations"

// Sentinel errors
var (
	ErrQueryFailed = errors.New("failed to query delegations")
)

type TezosGetDelegations struct {
	finder tezos.DelegationsFinder
}

func NewTezosGetDelegations(finder tezos.DelegationsFinder) *TezosGetDelegations {
	return &TezosGetDelegations{
		finder: finder,
	}
}

func (h *TezosGetDelegations) AddRoutes(m *http.ServeMux) {
	m.Handle(GetDelegationsRoute, httpkit.HandlerFunc(h.GetDelegations))
}

func (h *TezosGetDelegations) GetDelegations(w http.ResponseWriter, r *http.Request) http.HandlerFunc {
	// Parse query parameters using bind layer
	req, err := bind.GetDelegationsRequest(r)
	if err != nil {
		return httpkit.JsonError(api.BadRequest(err))
	}

	// Create domain criteria with validation
	criteria, err := tezos.NewDelegationsCriteria(req.Year, req.Page, req.PerPage)
	if err != nil {
		return httpkit.JsonError(api.BadRequest(err))
	}

	// Query delegations
	page, err := h.finder.FindDelegations(r.Context(), criteria)
	if err != nil {
		return httpkit.JsonError(api.InternalServerError(fmt.Errorf("%w: %w", ErrQueryFailed, err)))
	}

	// Build GitHub-style Link header for navigation
	if linkHeader := buildPaginationLinks(page, r.URL); linkHeader != "" {
		w.Header().Set("Link", linkHeader)
	}

	// Return JSON response
	resp := bind.GetDelegationsResponse(page.Delegations)
	return httpkit.JSON(resp)
}

// buildPaginationLinks creates GitHub-style Link header for pagination navigation
func buildPaginationLinks(page *tezos.DelegationsPage, baseURL *url.URL) string {
	var links []string

	// Build base URL with existing query params (like year filter)
	u := *baseURL
	query := u.Query()

	// Previous page link
	if page.HasPrevious() {
		query.Set("page", fmt.Sprintf("%d", page.Number-1))
		query.Set("per_page", fmt.Sprintf("%d", page.Size))
		u.RawQuery = query.Encode()
		links = append(links, fmt.Sprintf(`<%s>; rel="prev"`, u.String()))
	}

	// Next page link (GitHub-style: only if we know there are more pages)
	if page.HasNext() {
		query.Set("page", fmt.Sprintf("%d", page.Number+1))
		query.Set("per_page", fmt.Sprintf("%d", page.Size))
		u.RawQuery = query.Encode()
		links = append(links, fmt.Sprintf(`<%s>; rel="next"`, u.String()))
	}

	// Note: We intentionally omit "first" and "last" links for simplicity and performance.
	// rel="first" is redundant (always page=1) and rel="last" requires expensive count(*) queries.
	// Essential navigation (prev/next) works perfectly without the overhead.

	return strings.Join(links, ", ")
}
