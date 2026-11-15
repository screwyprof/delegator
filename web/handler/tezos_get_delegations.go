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
	finder    tezos.DelegationsFinder
	publicURL *url.URL
}

// Option configures optional TezosGetDelegations behaviour
type Option func(*TezosGetDelegations)

// WithPublicURL sets the base URL the API is published at (e.g. https://api.example.com),
// used for absolute Link URLs. A nil or empty URL is ignored (links use the request's host).
func WithPublicURL(u *url.URL) Option {
	return func(h *TezosGetDelegations) {
		if u != nil && u.Host != "" {
			h.publicURL = u
		}
	}
}

func NewTezosGetDelegations(finder tezos.DelegationsFinder, opts ...Option) *TezosGetDelegations {
	h := &TezosGetDelegations{
		finder: finder,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
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

	after, err := bind.DecodeCursor(req.After, req.Year)
	if err != nil {
		return httpkit.JsonError(api.BadRequest(err))
	}

	before, err := bind.DecodeCursor(req.Before, req.Year)
	if err != nil {
		return httpkit.JsonError(api.BadRequest(err))
	}

	// Create domain criteria with validation
	criteria, err := tezos.NewDelegationsCriteria(
		req.Year,
		req.Page,
		req.PerPage,
		tezos.After(after),
		tezos.Before(before),
	)
	if err != nil {
		return httpkit.JsonError(api.BadRequest(err))
	}

	// Query delegations
	page, err := h.finder.FindDelegations(r.Context(), criteria)
	if err != nil {
		return httpkit.JsonError(api.InternalServerError(fmt.Errorf("%w: %w", ErrQueryFailed, err)))
	}

	// Build GitHub-style Link header for navigation
	if linkHeader := buildPaginationLinks(page, h.requestURL(r), criteria.Year.Uint64()); linkHeader != "" {
		w.Header().Set("Link", linkHeader)
	}

	// Return JSON response
	resp := bind.GetDelegationsResponse(page.Delegations)
	return httpkit.JSON(resp)
}

// buildPaginationLinks creates a GitHub-style Link header for pagination navigation.
// Like GitHub, requests may use page numbers, but prev/next links always carry cursors: a page
// is anchored to a delegation's position rather than a row count, so following links never
// skips or repeats delegations, even while new ones are being stored. The end of the
// collection is signalled only by the absence of rel="next". rel="last" is omitted: it would
// need a count(*) over the whole result set, and cursors make it unnecessary.
func buildPaginationLinks(page *tezos.DelegationsPage, baseURL *url.URL, year uint64) string {
	var links []string

	if page.HasFirst() {
		links = append(links, fmt.Sprintf(`<%s>; rel="first"`, pageLink(baseURL, page.Size, "", "")))
	}

	if page.HasPrevious() {
		links = append(
			links,
			fmt.Sprintf(
				`<%s>; rel="prev"`,
				pageLink(baseURL, page.Size, "before", bind.EncodeCursor(page.PreviousCursor(), year)),
			),
		)
	}

	if page.HasNext() {
		links = append(
			links,
			fmt.Sprintf(
				`<%s>; rel="next"`,
				pageLink(baseURL, page.Size, "after", bind.EncodeCursor(page.NextCursor(), year)),
			),
		)
	}

	return strings.Join(links, ", ")
}

// pageLink is baseURL with its other query params (like year) kept, per_page made explicit,
// and page/after/before replaced by direction=token (or dropped, for the first page).
func pageLink(baseURL *url.URL, size tezos.PerPage, direction, token string) string {
	u := *baseURL
	query := u.Query()

	query.Del("page")
	query.Del("after")
	query.Del("before")
	query.Set("per_page", fmt.Sprintf("%d", size))
	if direction != "" {
		query.Set(direction, token)
	}

	u.RawQuery = query.Encode()
	return u.String()
}

// requestURL is the absolute URL of r as clients address it, so Link targets are absolute
// like GitHub's. With a public URL configured, its scheme, host and path prefix replace those
// r arrived with (e.g. behind a proxy that rewrites Host). Without one, it's the request's own
// Host and TLS-derived scheme. Host is client-supplied too, but it's part of every HTTP
// cache's key, so a spoofed Host only affects that client's own response. X-Forwarded-*
// headers are never consulted: caches don't key on them, so a spoofed value could end up in
// links a shared cache serves to other clients. Set WEB_PUBLIC_URL to not depend on Host.
func (h *TezosGetDelegations) requestURL(r *http.Request) *url.URL {
	u := *r.URL

	if h.publicURL != nil {
		u.Scheme = h.publicURL.Scheme
		u.Host = h.publicURL.Host
		u.Path = strings.TrimSuffix(h.publicURL.Path, "/") + r.URL.Path
		u.RawPath = ""
		return &u
	}

	u.Host = r.Host
	u.Scheme = "http"
	if r.TLS != nil {
		u.Scheme = "https"
	}
	return &u
}
