//go:build acceptance

package web_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/screwyprof/delegator/migrator/migratortest"
	"github.com/screwyprof/delegator/pkg/logger"
	"github.com/screwyprof/delegator/pkg/pgxdb"
	"github.com/screwyprof/delegator/web/api"
	"github.com/screwyprof/delegator/web/config"
	"github.com/screwyprof/delegator/web/handler"
	"github.com/screwyprof/delegator/web/internal/seedtestdb"
	"github.com/screwyprof/delegator/web/store/pgxstore"
	"github.com/screwyprof/delegator/web/tezos"
)

// TestWebAPIAcceptanceBehavior tests end-to-end web API functionality.
func TestWebAPIAcceptanceBehavior(t *testing.T) {
	t.Parallel()

	// Create ONE shared read-only test database for all subtests
	// Since we never modify data, this can be safely shared
	sharedTestDB := seedtestdb.CreateSeededTestDatabase(t, "../migrator/migrations")
	t.Cleanup(func() {
		sharedTestDB.Close()
	})

	// Get the connection string to the shared database
	dbConnString := sharedTestDB.Config().ConnString()

	t.Run("it returns delegations with default pagination and ordering", func(t *testing.T) {
		t.Parallel()

		// Arrange
		server, cleanup := createTestServerUsingSeededDatabase(t, dbConnString)
		defer cleanup()

		client := createTestAPIClient(t)

		// Act
		response := makeGetDelegations(t, client, server.URL, delegationsQuery{})
		delegationsResp := parseJSONResponse[api.DelegationsResponse](t, response)

		// Assert
		assertSuccessfulResponse(t, response)
		assertReturnsDefaultPagination(t, delegationsResp)
		assertDelegationsOrderedMostRecentFirst(t, delegationsResp.Data)
		assertAllDelegationsHaveValidFormat(t, delegationsResp.Data)

		t.Logf("✅ Default pagination test completed successfully")
	})

	t.Run("it filters delegations by year parameter", func(t *testing.T) {
		t.Parallel()

		// Arrange
		const year = 2025

		server, cleanup := createTestServerUsingSeededDatabase(t, dbConnString)
		defer cleanup()

		client := createTestAPIClient(t)

		// Act
		response := makeGetDelegations(t, client, server.URL, delegationsQuery{Year: year})
		delegationsResp := parseJSONResponse[api.DelegationsResponse](t, response)

		// Assert
		assertSuccessfulResponse(t, response)
		assertReturnsNonEmptyResults(t, delegationsResp)
		assertAllDelegationsFromYear(t, delegationsResp.Data, year)

		t.Logf("✅ Year filtering test completed successfully")
	})

	t.Run("it provides GitHub-style pagination Link headers", func(t *testing.T) {
		t.Parallel()

		t.Run("it omits Link header when results fit on first page", func(t *testing.T) {
			t.Parallel()

			// Arrange
			server, cleanup := createTestServerWithMinimalData(t)
			defer cleanup()

			client := createTestAPIClient(t)

			// Act
			response := makeGetDelegations(t, client, server.URL, delegationsQuery{})
			delegationsResp := parseJSONResponse[api.DelegationsResponse](t, response)

			// Assert
			assertSuccessfulResponse(t, response)
			assertExactDelegationCount(t, delegationsResp, 2)
			assertPaginationLinksAbsent(t, response)
		})

		t.Run("it provides next link on first page when more pages exist", func(t *testing.T) {
			t.Parallel()

			// Arrange
			server, cleanup := createTestServerUsingSeededDatabase(t, dbConnString)
			defer cleanup()

			client := createTestAPIClient(t)

			// Act
			response := makeGetDelegations(t, client, server.URL, delegationsQuery{Page: 1, PerPage: 10})

			// Assert
			assertSuccessfulResponse(t, response)
			assertPaginationLinksPresent(t, response)
			assertContainsNextLink(t, response)
			assertMissingPrevLink(t, response)
		})

		t.Run("it provides navigation links on middle pages", func(t *testing.T) {
			t.Parallel()

			// Arrange
			server, cleanup := createTestServerUsingSeededDatabase(t, dbConnString)
			defer cleanup()

			client := createTestAPIClient(t)

			// Act
			response := makeGetDelegations(t, client, server.URL, delegationsQuery{Page: 2, PerPage: 10})

			// Assert
			assertSuccessfulResponse(t, response)
			assertPaginationLinksPresent(t, response)
			assertContainsPrevLink(t, response)
			assertCursorNavigation(t, response, 10)
		})

		t.Run("it builds absolute links from the configured public URL", func(t *testing.T) {
			t.Parallel()

			// The API is published at a different origin than it's reached on (behind a proxy),
			// optionally under a path prefix the proxy strips
			for publicBase, wantLinkBase := range map[string]string{
				"https://api.example.com":       "https://api.example.com/xtz/delegations",
				"https://example.com/tezos-v1/": "https://example.com/tezos-v1/xtz/delegations",
			} {
				// Arrange
				publicURL, err := url.Parse(publicBase)
				require.NoError(t, err)

				server, cleanup := createTestServerUsingSeededDatabase(
					t,
					dbConnString,
					handler.WithPublicURL(publicURL),
				)
				client := createTestAPIClient(t)

				// Act
				response := makeGetDelegations(t, client, server.URL, delegationsQuery{Page: 2, PerPage: 10})

				// Assert
				assertSuccessfulResponse(t, response)

				for _, rel := range []string{"first", "prev", "next"} {
					link := linkURL(response, rel)
					require.NotNil(t, link, "Should provide a %s link", rel)
					assert.Equal(t, wantLinkBase, link.Scheme+"://"+link.Host+link.Path,
						"%s link should use the public URL %s, not the host the request arrived on", rel, publicBase)
				}

				cleanup()
			}
		})

		t.Run("it ignores client-supplied forwarded headers when building links", func(t *testing.T) {
			t.Parallel()

			// Arrange
			server, cleanup := createTestServerUsingSeededDatabase(t, dbConnString)
			defer cleanup()

			client := createTestAPIClient(t)

			req, err := http.NewRequestWithContext(
				t.Context(),
				http.MethodGet,
				server.URL+"/xtz/delegations?per_page=10",
				nil,
			)
			require.NoError(t, err, "Should create HTTP request")
			req.Header.Set("X-Forwarded-Proto", "https")
			req.Header.Set("X-Forwarded-Host", "evil.example")

			// Act
			response, err := client.Do(req)
			require.NoError(t, err, "HTTP request should succeed")

			defer func() { _ = response.Body.Close() }()

			// Assert: a spoofed header must not reach links (a shared cache could serve them to others)
			next := linkURL(response, "next")
			require.NotNil(t, next, "Should provide a next link")
			assert.Equal(t, server.URL, next.Scheme+"://"+next.Host,
				"Without a configured public URL, links should use the actual request scheme and host")
		})

		t.Run("it links back to the first page from a page past the end", func(t *testing.T) {
			t.Parallel()

			// Arrange
			server, cleanup := createTestServerUsingSeededDatabase(t, dbConnString)
			defer cleanup()

			client := createTestAPIClient(t)

			// Act
			response := makeGetDelegations(
				t,
				client,
				server.URL,
				delegationsQuery{Page: tezos.MaxPageOffset / 10, PerPage: 10},
			)
			delegationsResp := parseJSONResponse[api.DelegationsResponse](t, response)

			// Assert
			assertSuccessfulResponse(t, response)
			assertExactDelegationCount(t, delegationsResp, 0)

			first := linkURL(response, "first")
			require.NotNil(t, first, "An empty page past the end should still link back to the first page")
			assert.False(t, first.Query().Has("page"), "first link should not carry page")
		})

		t.Run("it rejects page numbers past the depth cap, including overflowing ones", func(t *testing.T) {
			t.Parallel()

			// Arrange
			server, cleanup := createTestServerUsingSeededDatabase(t, dbConnString)
			defer cleanup()

			client := createTestAPIClient(t)

			for name, rawQuery := range map[string]string{
				"largest uint64 page (was a 500)":        "page=18446744073709551615",
				"offset wrapping to 0 (was page 1, 200)": "page=288230376151711745&per_page=64",
				"one page past the depth cap":            "page=1002&per_page=100",
			} {
				// Act
				req, err := http.NewRequestWithContext(t.Context(), http.MethodGet,
					server.URL+"/xtz/delegations?"+rawQuery, nil)
				require.NoError(t, err, "Should create HTTP request")
				response, err := client.Do(req)
				require.NoError(t, err, "HTTP request should succeed")

				_ = response.Body.Close()

				// Assert
				assert.Equal(t, http.StatusBadRequest, response.StatusCode, name)
			}
		})

		t.Run("it rejects a cursor whose position is out of range", func(t *testing.T) {
			t.Parallel()

			// Arrange
			server, cleanup := createTestServerUsingSeededDatabase(t, dbConnString)
			defer cleanup()

			client := createTestAPIClient(t)
			forged := base64.RawURLEncoding.EncodeToString([]byte("v1:0:-9223372036854775808:1"))

			// Act
			req, err := http.NewRequestWithContext(
				t.Context(),
				http.MethodGet,
				server.URL+"/xtz/delegations?after="+forged,
				nil,
			)
			require.NoError(t, err, "Should create HTTP request")
			response, err := client.Do(req)
			require.NoError(t, err, "HTTP request should succeed")

			defer func() { _ = response.Body.Close() }()

			// Assert
			assert.Equal(
				t,
				http.StatusBadRequest,
				response.StatusCode,
				"A forged cursor outside any representable timestamp must be rejected, "+
					"not silently page a wrapped-around position",
			)
		})

		t.Run("it rejects a cursor replayed under a different year filter", func(t *testing.T) {
			t.Parallel()

			// Arrange
			server, cleanup := createTestServerUsingSeededDatabase(t, dbConnString)
			defer cleanup()

			client := createTestAPIClient(t)

			firstPage := makeGetDelegations(t, client, server.URL, delegationsQuery{Year: 2025, Page: 1, PerPage: 10})
			next := linkURL(firstPage, "next")
			require.NotNil(t, next, "Should provide a next link")

			query := next.Query()
			query.Set("year", "2024")
			next.RawQuery = query.Encode()

			// Act
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, next.String(), nil)
			require.NoError(t, err, "Should create HTTP request")
			response, err := client.Do(req)
			require.NoError(t, err, "HTTP request should succeed")

			defer func() { _ = response.Body.Close() }()

			// Assert
			assert.Equal(t, http.StatusBadRequest, response.StatusCode,
				"A cursor only identifies a position within the result set it was issued for")
		})

		t.Run("it returns the previous page when following a prev link", func(t *testing.T) {
			t.Parallel()

			// Arrange
			const perPage = 3

			server, db, cleanup := createTestServerWithCleanDatabase(t)
			defer cleanup()

			insertDelegationsWithTiedTimestamps(t, db)
			client := createTestAPIClient(t)

			firstPage := makeGetDelegations(t, client, server.URL, delegationsQuery{Page: 1, PerPage: perPage})
			secondPage := followLink(t, client, firstPage, "next")
			_ = parseJSONResponse[api.DelegationsResponse](t, firstPage)

			// Act
			backToFirst := followLink(t, client, secondPage, "prev")
			delegationsResp := parseJSONResponse[api.DelegationsResponse](t, backToFirst)

			// Assert
			assertSuccessfulResponse(t, backToFirst)
			assert.Equal(t, tiedDelegationsNewestFirst()[:perPage], delegatorIDs(t, delegationsResp.Data),
				"Following prev from the second page should return exactly the first page")
			assertMissingPrevLink(t, backToFirst)
			assertContainsNextLink(t, backToFirst)
		})

		t.Run("it breaks timestamp ties by id, newest first", func(t *testing.T) {
			t.Parallel()

			// Arrange
			server, db, cleanup := createTestServerWithCleanDatabase(t)
			defer cleanup()

			insertDelegationsWithTiedTimestamps(t, db)
			client := createTestAPIClient(t)

			// Act
			response := makeGetDelegations(t, client, server.URL, delegationsQuery{Page: 1, PerPage: tezos.MaxPerPage})
			delegationsResp := parseJSONResponse[api.DelegationsResponse](t, response)

			// Assert
			assertSuccessfulResponse(t, response)
			assert.Equal(t, tiedDelegationsNewestFirst(), delegatorIDs(t, delegationsResp.Data),
				"Delegations sharing a timestamp should come back in a stable id-descending order")
		})

		t.Run(
			"it returns every delegation exactly once when following next links while new ones arrive",
			func(t *testing.T) {
				t.Parallel()

				// Arrange
				const perPage = 2

				server, db, cleanup := createTestServerWithCleanDatabase(t)
				defer cleanup()

				insertDelegationsWithTiedTimestamps(t, db)
				client := createTestAPIClient(t)

				// Act
				seen := walkNextLinks(t, client, makeGetDelegationsURL(server.URL, perPage), func() {
					insertNewerDelegation(t, db) // the scraper lands a new delegation mid-walk
				})

				// Assert
				assert.Equal(
					t,
					tiedDelegationsNewestFirst(),
					seen,
					"Following next links should return each pre-existing delegation once, in order, "+
						"with no gaps or repeats",
				)
			},
		)

		t.Run("it preserves query parameters in pagination links", func(t *testing.T) {
			t.Parallel()

			// Arrange
			const (
				year    = 2025
				perPage = 5
				page    = 2
			)

			server, cleanup := createTestServerUsingSeededDatabase(t, dbConnString)
			defer cleanup()

			client := createTestAPIClient(t)

			// Act
			response := makeGetDelegations(
				t,
				client,
				server.URL,
				delegationsQuery{Year: year, Page: page, PerPage: perPage},
			)

			// Assert
			assertSuccessfulResponse(t, response)
			assertPaginationLinksPresent(t, response)
			assertPreservesQueryParameters(t, response, map[string]string{
				"year":     strconv.Itoa(year),
				"per_page": strconv.Itoa(perPage),
			})
		})
	})
}

// =============================================================================
// Arrange Phase Helpers - Factory functions for test setup
// =============================================================================

// createTestAPIClient creates an HTTP client for API testing.
func createTestAPIClient(t *testing.T) *http.Client {
	t.Helper()

	return http.DefaultClient
}

// createTestServerUsingSeededDatabase creates a test server that connects to an already-seeded database.
func createTestServerUsingSeededDatabase(
	t *testing.T,
	dbConnString string,
	opts ...handler.Option,
) (*httptest.Server, func()) {
	t.Helper()

	return createTestServerWithIsolatedConnection(t, dbConnString, opts...)
}

// createTestServerWithMinimalData creates a test server with minimal test data.
func createTestServerWithMinimalData(t *testing.T) (*httptest.Server, func()) {
	t.Helper()

	// Create clean database with only schema (no seeded data)
	cleanTestDB := migratortest.CreateScraperTestDatabase(t, "../migrator/migrations", 0)
	t.Cleanup(func() {
		cleanTestDB.Close()
	})

	// Manually insert just 2 test delegations (fits in one page)
	insertTestDelegations(t, cleanTestDB)

	// Create server with clean database
	return createTestServerWithIsolatedConnection(t, cleanTestDB.Config().ConnString())
}

// =============================================================================
// Action Helpers - HTTP request helpers that express intent
// =============================================================================

// delegationsQuery is the query string of a GET /xtz/delegations request; zero fields are omitted.
type delegationsQuery struct {
	Year, Page, PerPage int
}

// makeGetDelegations performs GET /xtz/delegations with the given query.
func makeGetDelegations(t *testing.T, client *http.Client, baseURL string, q delegationsQuery) *http.Response {
	t.Helper()

	query := url.Values{}

	for name, value := range map[string]int{"year": q.Year, "page": q.Page, "per_page": q.PerPage} {
		if value != 0 {
			query.Set(name, strconv.Itoa(value))
		}
	}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, baseURL+"/xtz/delegations?"+query.Encode(), nil)
	require.NoError(t, err, "Should create HTTP request")

	resp, err := client.Do(req)
	require.NoError(t, err, "HTTP request should succeed")

	return resp
}

// =============================================================================
// Named Domain Assertions - Business rule assertions
// =============================================================================

// assertSuccessfulResponse verifies the HTTP response indicates success.
func assertSuccessfulResponse(t *testing.T, resp *http.Response) {
	t.Helper()
	assert.Equal(t, http.StatusOK, resp.StatusCode, "Should return HTTP 200 OK")
}

// assertReturnsDefaultPagination verifies response uses default pagination (50 items).
func assertReturnsDefaultPagination(t *testing.T, response api.DelegationsResponse) {
	t.Helper()
	assert.Len(
		t,
		response.Data, tezos.DefaultPerPage,
		"Should return exactly %d delegations (default pagination limit)",
		tezos.DefaultPerPage,
	)
}

// assertReturnsNonEmptyResults verifies response contains at least one delegation.
func assertReturnsNonEmptyResults(t *testing.T, response api.DelegationsResponse) {
	t.Helper()
	assert.NotEmpty(t, response.Data, "Should return some delegations")
	t.Logf("📋 Response contains %d delegations", len(response.Data))
}

// assertExactDelegationCount verifies response contains exactly the expected number of delegations.
func assertExactDelegationCount(t *testing.T, response api.DelegationsResponse, expected int) {
	t.Helper()
	assert.Len(t, response.Data, expected, "Should return exactly %d delegations", expected)
}

// assertDelegationsOrderedMostRecentFirst verifies delegations are ordered by timestamp descending.
func assertDelegationsOrderedMostRecentFirst(t *testing.T, delegations []api.Delegation) {
	t.Helper()

	if len(delegations) <= 1 {
		return // Nothing to verify with 0 or 1 items
	}

	for i := range len(delegations) - 1 {
		current := delegations[i].Timestamp
		next := delegations[i+1].Timestamp
		assert.GreaterOrEqual(t, current, next,
			"Delegations should be ordered most recent first (index %d: %s should be >= %s)",
			i, current, next)
	}

	t.Logf("✅ Ordering verified: most recent first")
}

// assertAllDelegationsFromYear verifies all delegations are from the specified year.
func assertAllDelegationsFromYear(t *testing.T, delegations []api.Delegation, year int) {
	t.Helper()

	for i, delegation := range delegations {
		timestamp, err := time.Parse(time.RFC3339, delegation.Timestamp)
		require.NoError(t, err, "Should parse delegation timestamp")

		actualYear := timestamp.Year()
		assert.Equal(t, year, actualYear, "Delegation %d should be from year %d, got %d", i, year, actualYear)
	}
}

// assertAllDelegationsHaveValidFormat verifies all delegations match the expected format.
func assertAllDelegationsHaveValidFormat(t *testing.T, delegations []api.Delegation) {
	t.Helper()

	for i, delegation := range delegations {
		assertValidDelegationFormat(t, delegation, i)
	}
}

// assertValidDelegationFormat verifies a single delegation matches TASK.md specification.
func assertValidDelegationFormat(t *testing.T, delegation api.Delegation, index int) {
	t.Helper()

	assert.NotEmpty(t, delegation.Timestamp, "Delegation %d should have timestamp", index)
	assert.NotEmpty(t, delegation.Amount, "Delegation %d should have amount", index)
	assert.NotEmpty(t, delegation.Delegator, "Delegation %d should have delegator", index)
	assert.NotEmpty(t, delegation.Level, "Delegation %d should have level", index)
}

// assertPaginationLinksPresent verifies Link header is present.
func assertPaginationLinksPresent(t *testing.T, resp *http.Response) {
	t.Helper()

	linkHeader := resp.Header.Get("Link")
	assert.NotEmpty(t, linkHeader, "Should provide Link header when pagination is needed")
}

// assertPaginationLinksAbsent verifies Link header is absent.
func assertPaginationLinksAbsent(t *testing.T, resp *http.Response) {
	t.Helper()

	linkHeader := resp.Header.Get("Link")
	assert.Empty(t, linkHeader, "Should omit Link header when all results fit on first page")
}

// assertContainsNextLink verifies Link header contains next link.
func assertContainsNextLink(t *testing.T, resp *http.Response) {
	t.Helper()

	linkHeader := resp.Header.Get("Link")
	assert.Contains(t, linkHeader, `rel="next"`, "Should provide next link when more pages exist")
}

// assertMissingPrevLink verifies Link header does not contain prev link.
func assertMissingPrevLink(t *testing.T, resp *http.Response) {
	t.Helper()

	linkHeader := resp.Header.Get("Link")
	assert.NotContains(t, linkHeader, `rel="prev"`, "Should not provide prev link on first page")
}

// assertContainsPrevLink verifies Link header contains prev link.
func assertContainsPrevLink(t *testing.T, resp *http.Response) {
	t.Helper()

	linkHeader := resp.Header.Get("Link")
	assert.Contains(t, linkHeader, `rel="prev"`, "Should provide prev link when not on first page")
}

// assertCursorNavigation verifies links are absolute (like GitHub's), prev/next are anchored
// to cursors (before/after) rather than page numbers, first carries no position, and all
// keep per_page.
func assertCursorNavigation(t *testing.T, resp *http.Response, expectedPerPage int) {
	t.Helper()

	assert.Regexp(t, `^<https?://[^>]+>; rel="first"`, resp.Header.Get("Link"), "Link targets should be absolute URLs")

	first := linkURL(resp, "first")
	require.NotNil(t, first, "Should provide a first link when not on the first page")

	for _, param := range []string{"page", "after", "before"} {
		assert.False(t, first.Query().Has(param), "first link should not carry %s", param)
	}

	assert.Equal(t, strconv.Itoa(expectedPerPage), first.Query().Get("per_page"), "first link should preserve per_page")

	for rel, cursorParam := range map[string]string{"prev": "before", "next": "after"} {
		u := linkURL(resp, rel)
		require.NotNil(t, u, "Should provide a %s link", rel)

		query := u.Query()
		assert.NotEmpty(t, query.Get(cursorParam), "%s link should carry a %s cursor", rel, cursorParam)
		assert.False(t, query.Has("page"), "%s link should not use page numbers", rel)
		assert.Equal(t, strconv.Itoa(expectedPerPage), query.Get("per_page"), "%s link should preserve per_page", rel)
	}
}

// assertPreservesQueryParameters verifies pagination links preserve query parameters.
func assertPreservesQueryParameters(t *testing.T, resp *http.Response, expectedParams map[string]string) {
	t.Helper()

	linkHeader := resp.Header.Get("Link")
	assert.NotEmpty(t, linkHeader, "Should provide Link header on middle pages with parameters")

	for param, value := range expectedParams {
		expectedParam := fmt.Sprintf("%s=%s", param, value)
		assert.Contains(t, linkHeader, expectedParam, "Should preserve %s parameter in navigation links", param)
	}
}

// =============================================================================
// Utility Functions
// =============================================================================

// parseJSONResponse parses HTTP response body as JSON into the specified type.
func parseJSONResponse[T any](t *testing.T, resp *http.Response) T {
	t.Helper()

	defer func() { _ = resp.Body.Close() }()

	var result T

	err := json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err, "Response should be valid JSON")

	return result
}

// insertTestDelegations manually inserts a few test delegations for Link header omission test.
func insertTestDelegations(t *testing.T, db *pgxpool.Pool) {
	t.Helper()

	ctx := t.Context()

	// Insert 2 test delegations that will fit on one page
	insertSQL := `
		INSERT INTO delegations (id, timestamp, amount, delegator, level, year) 
		VALUES 
			(1, '2025-01-15T10:30:00Z', 1000000, 'tz1TestDelegator1', 4500000, 2025),
			(2, '2025-01-14T15:45:00Z', 2000000, 'tz1TestDelegator2', 4499999, 2025)
	`

	_, err := db.Exec(ctx, insertSQL)
	require.NoError(t, err, "Should insert test delegations")
}

// tiedDelegationsNewestFirst is the id order the API must return insertDelegationsWithTiedTimestamps'
// rows in: timestamp DESC, then id DESC. Ids 9..5 share one timestamp, so with small pages
// the tie straddles page boundaries.
func tiedDelegationsNewestFirst() []int {
	return []int{10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
}

// createTestServerWithCleanDatabase creates a test server over an empty, schema-only database
// and also returns a pool to it, so a test can insert data (including mid-request-sequence).
func createTestServerWithCleanDatabase(t *testing.T) (*httptest.Server, *pgxpool.Pool, func()) {
	t.Helper()

	db := migratortest.CreateScraperTestDatabase(t, "../migrator/migrations", 0)
	server, cleanup := createTestServerWithIsolatedConnection(t, db.Config().ConnString())

	return server, db, func() {
		cleanup()
		db.Close()
	}
}

// insertDelegationsWithTiedTimestamps inserts 10 delegations, ids 1..10, in ascending id order.
// Ids 5..9 share a timestamp. Each delegator is "tz1Test<id>" so tests can recover the id
// from API responses, which don't expose it.
func insertDelegationsWithTiedTimestamps(t *testing.T, db *pgxpool.Pool) {
	t.Helper()

	_, err := db.Exec(t.Context(), `
		INSERT INTO delegations (id, timestamp, amount, delegator, level, year)
		SELECT id,
		       CASE WHEN id BETWEEN 5 AND 9 THEN '2025-01-15T10:00:05Z'::timestamptz
		            WHEN id = 10            THEN '2025-01-15T10:00:06Z'::timestamptz
		            ELSE '2025-01-15T10:00:00Z'::timestamptz + id * interval '1 second' END,
		       1000000, 'tz1Test' || id, 4500000 + id, 2025
		FROM generate_series(1, 10) AS id ORDER BY id`)
	require.NoError(t, err, "Should insert delegations with tied timestamps")
}

// insertNewerDelegation simulates the scraper storing a delegation newer than everything else.
func insertNewerDelegation(t *testing.T, db *pgxpool.Pool) {
	t.Helper()

	_, err := db.Exec(t.Context(), `
		INSERT INTO delegations (id, timestamp, amount, delegator, level, year)
		VALUES (11, '2025-01-15T10:00:07Z', 1000000, 'tz1Test11', 4500011, 2025)`)
	require.NoError(t, err, "Should insert a newer delegation")
}

// makeGetDelegationsURL builds the first-page URL for a pagination walk.
func makeGetDelegationsURL(baseURL string, perPage int) string {
	return fmt.Sprintf("%s/xtz/delegations?per_page=%d", baseURL, perPage)
}

// walkNextLinks GETs url, then keeps following rel="next" links until there are none,
// returning the ids of every delegation seen in order. afterFirstPage runs once, between
// the first and second request, to change the data mid-walk.
func walkNextLinks(t *testing.T, client *http.Client, url string, afterFirstPage func()) []int {
	t.Helper()

	var seen []int

	for page := 1; url != ""; page++ {
		require.LessOrEqual(t, page, 100, "Pagination walk should terminate")

		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
		require.NoError(t, err, "Should create HTTP request")
		resp, err := client.Do(req)
		require.NoError(t, err, "HTTP request should succeed")

		assertSuccessfulResponse(t, resp)
		url = nextLink(resp)
		seen = append(seen, delegatorIDs(t, parseJSONResponse[api.DelegationsResponse](t, resp).Data)...)

		if page == 1 {
			afterFirstPage()
		}
	}

	return seen
}

// followLink GETs the URL of resp's Link with the given rel, which must exist.
func followLink(t *testing.T, client *http.Client, resp *http.Response, rel string) *http.Response {
	t.Helper()

	u := linkURL(resp, rel)
	require.NotNil(t, u, "Should provide a %s link", rel)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, u.String(), nil)
	require.NoError(t, err, "Should create HTTP request")
	next, err := client.Do(req)
	require.NoError(t, err, "HTTP request should succeed")

	return next
}

// nextLink extracts the rel="next" URL from a Link header, or "" if there is none.
func nextLink(resp *http.Response) string {
	if u := linkURL(resp, "next"); u != nil {
		return u.String()
	}

	return ""
}

// linkURL extracts the URL with the given rel from a Link header, resolved against the
// request URL (link targets may be relative), or nil if there is none.
func linkURL(resp *http.Response, rel string) *url.URL {
	for link := range strings.SplitSeq(resp.Header.Get("Link"), ",") {
		target, params, ok := strings.Cut(strings.TrimSpace(link), ";")
		if ok && strings.TrimSpace(params) == fmt.Sprintf("rel=%q", rel) {
			u, err := resp.Request.URL.Parse(strings.Trim(target, "<>"))
			if err != nil {
				return nil
			}

			return u
		}
	}

	return nil
}

// delegatorIDs recovers the ids of delegations inserted by this file's helpers ("tz1Test<id>").
func delegatorIDs(t *testing.T, delegations []api.Delegation) []int {
	t.Helper()

	ids := make([]int, 0, len(delegations))
	for _, d := range delegations {
		id, err := strconv.Atoi(strings.TrimPrefix(d.Delegator, "tz1Test"))
		require.NoError(t, err, "Delegator %q should be a tz1Test<id> fixture", d.Delegator)

		ids = append(ids, id)
	}

	return ids
}

// createTestServerWithIsolatedConnection creates a test server with its own connection pool
// to the provided database. Each test gets isolated connection resources but shares the read-only database.
func createTestServerWithIsolatedConnection(
	t *testing.T,
	dbConnString string,
	opts ...handler.Option,
) (*httptest.Server, func()) {
	t.Helper()

	// Each test gets its own connection pool to the shared read-only database
	storeConn, err := pgxdb.NewConnection(t.Context(), dbConnString)
	require.NoError(t, err)

	// Each test gets its own store
	store := pgxstore.New(storeConn)

	// Create server with isolated connection resources and logging (like production)
	mux := http.NewServeMux()
	tezosHandler := handler.NewTezosGetDelegations(store, opts...)
	tezosHandler.AddRoutes(mux)

	// Add logging middleware for SUT observability (like production) — same env vars
	// production uses (LOG_LEVEL/LOG_HUMAN_FRIENDLY), no test-only copy. Note this means
	// test output defaults to JSON (LogHumanFriendly's production-safe default is false);
	// export LOG_HUMAN_FRIENDLY=true locally for readable logs while debugging a failure.
	cfg := config.New()
	log := logger.NewFromConfig(logger.Config{
		LogLevel:         cfg.LogLevel,
		LogHumanFriendly: cfg.LogHumanFriendly,
	})
	loggedMux := logger.NewMiddleware(log)(mux)

	server := httptest.NewServer(loggedMux)

	// Return server and cleanup function for this test's resources
	cleanup := func() {
		server.Close()
		storeConn.Close() // the connection pool this test opened
	}

	return server, cleanup
}
