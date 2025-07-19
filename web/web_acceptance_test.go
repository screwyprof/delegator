////go:build acceptance

package web_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/screwyprof/delegator/migrator/migratortest"
	"github.com/screwyprof/delegator/pkg/logger"
	"github.com/screwyprof/delegator/pkg/pgxdb"
	"github.com/screwyprof/delegator/web/api"
	"github.com/screwyprof/delegator/web/handler"
	"github.com/screwyprof/delegator/web/store/pgxstore"
	"github.com/screwyprof/delegator/web/testcfg"
	"github.com/screwyprof/delegator/web/tezos"
)

// TestWebAPIAcceptanceBehavior tests end-to-end web API functionality
func TestWebAPIAcceptanceBehavior(t *testing.T) {
	t.Parallel()

	// Create ONE shared read-only test database for all subtests
	// Since we never modify data, this can be safely shared
	sharedTestDB := migratortest.CreateSeededTestDatabase(t, "../migrator/migrations")
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
		response := makeGetDelegationsRequest(t, client, server.URL)
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
		response := makeGetDelegationsWithYearRequest(t, client, server.URL, year)
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
			response := makeGetDelegationsRequest(t, client, server.URL)
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
			response := makeGetDelegationsWithPagination(t, client, server.URL, 1, 10)

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
			response := makeGetDelegationsWithPagination(t, client, server.URL, 2, 10)

			// Assert
			assertSuccessfulResponse(t, response)
			assertPaginationLinksPresent(t, response)
			assertContainsPrevLink(t, response)
			assertCorrectPageNavigation(t, response, 1, 10)
		})

		t.Run("it preserves query parameters in pagination links", func(t *testing.T) {
			t.Parallel()

			// Arrange
			const year = 2025
			const perPage = 5
			const page = 2

			server, cleanup := createTestServerUsingSeededDatabase(t, dbConnString)
			defer cleanup()
			client := createTestAPIClient(t)

			// Act
			response := makeGetDelegationsWithYearAndPagination(t, client, server.URL, year, page, perPage)

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

// createTestAPIClient creates an HTTP client for API testing
func createTestAPIClient(t *testing.T) *http.Client {
	t.Helper()
	return http.DefaultClient
}

// createTestServerUsingSeededDatabase creates a test server that connects to an already-seeded database
func createTestServerUsingSeededDatabase(t *testing.T, dbConnString string) (*httptest.Server, func()) {
	t.Helper()
	return createTestServerWithIsolatedConnection(t, dbConnString)
}

// createTestServerWithMinimalData creates a test server with minimal test data
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

// makeGetDelegationsRequest performs a basic GET /xtz/delegations request
func makeGetDelegationsRequest(t *testing.T, client *http.Client, baseURL string) *http.Response {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, baseURL+"/xtz/delegations", nil)
	require.NoError(t, err, "Should create HTTP request")

	resp, err := client.Do(req)
	require.NoError(t, err, "HTTP request should succeed")

	return resp
}

// makeGetDelegationsWithYearRequest performs GET /xtz/delegations with year filter
func makeGetDelegationsWithYearRequest(t *testing.T, client *http.Client, baseURL string, year int) *http.Response {
	t.Helper()

	url := fmt.Sprintf("%s/xtz/delegations?year=%d", baseURL, year)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	require.NoError(t, err, "Should create HTTP request")

	resp, err := client.Do(req)
	require.NoError(t, err, "HTTP request should succeed")

	return resp
}

// makeGetDelegationsWithPagination performs GET /xtz/delegations with pagination
func makeGetDelegationsWithPagination(t *testing.T, client *http.Client, baseURL string, page, perPage int) *http.Response {
	t.Helper()

	url := fmt.Sprintf("%s/xtz/delegations?page=%d&per_page=%d", baseURL, page, perPage)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	require.NoError(t, err, "Should create HTTP request")

	resp, err := client.Do(req)
	require.NoError(t, err, "HTTP request should succeed")

	return resp
}

// makeGetDelegationsWithYearAndPagination performs GET /xtz/delegations with year filter and pagination
func makeGetDelegationsWithYearAndPagination(t *testing.T, client *http.Client, baseURL string, year, page, perPage int) *http.Response {
	t.Helper()

	url := fmt.Sprintf("%s/xtz/delegations?year=%d&page=%d&per_page=%d", baseURL, year, page, perPage)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	require.NoError(t, err, "Should create HTTP request")

	resp, err := client.Do(req)
	require.NoError(t, err, "HTTP request should succeed")

	return resp
}

// =============================================================================
// Named Domain Assertions - Business rule assertions
// =============================================================================

// assertSuccessfulResponse verifies the HTTP response indicates success
func assertSuccessfulResponse(t *testing.T, resp *http.Response) {
	t.Helper()
	assert.Equal(t, http.StatusOK, resp.StatusCode, "Should return HTTP 200 OK")
}

// assertReturnsDefaultPagination verifies response uses default pagination (50 items)
func assertReturnsDefaultPagination(t *testing.T, response api.DelegationsResponse) {
	t.Helper()
	assert.Equal(t, tezos.DefaultPerPage, len(response.Data), "Should return exactly %d delegations (default pagination limit)", tezos.DefaultPerPage)
}

// assertReturnsNonEmptyResults verifies response contains at least one delegation
func assertReturnsNonEmptyResults(t *testing.T, response api.DelegationsResponse) {
	t.Helper()
	assert.Greater(t, len(response.Data), 0, "Should return some delegations")
	t.Logf("📋 Response contains %d delegations", len(response.Data))
}

// assertExactDelegationCount verifies response contains exactly the expected number of delegations
func assertExactDelegationCount(t *testing.T, response api.DelegationsResponse, expected int) {
	t.Helper()
	assert.Equal(t, expected, len(response.Data), "Should return exactly %d delegations", expected)
}

// assertDelegationsOrderedMostRecentFirst verifies delegations are ordered by timestamp descending
func assertDelegationsOrderedMostRecentFirst(t *testing.T, delegations []api.Delegation) {
	t.Helper()

	if len(delegations) <= 1 {
		return // Nothing to verify with 0 or 1 items
	}

	for i := 0; i < len(delegations)-1; i++ {
		current := delegations[i].Timestamp
		next := delegations[i+1].Timestamp
		assert.GreaterOrEqual(t, current, next,
			"Delegations should be ordered most recent first (index %d: %s should be >= %s)",
			i, current, next)
	}
	t.Logf("✅ Ordering verified: most recent first")
}

// assertAllDelegationsFromYear verifies all delegations are from the specified year
func assertAllDelegationsFromYear(t *testing.T, delegations []api.Delegation, year int) {
	t.Helper()

	for i, delegation := range delegations {
		timestamp, err := time.Parse(time.RFC3339, delegation.Timestamp)
		require.NoError(t, err, "Should parse delegation timestamp")

		actualYear := timestamp.Year()
		assert.Equal(t, year, actualYear, "Delegation %d should be from year %d, got %d", i, year, actualYear)
	}
}

// assertAllDelegationsHaveValidFormat verifies all delegations match the expected format
func assertAllDelegationsHaveValidFormat(t *testing.T, delegations []api.Delegation) {
	t.Helper()

	for i, delegation := range delegations {
		assertValidDelegationFormat(t, delegation, i)
	}
}

// assertValidDelegationFormat verifies a single delegation matches TASK.md specification
func assertValidDelegationFormat(t *testing.T, delegation api.Delegation, index int) {
	t.Helper()

	assert.NotEmpty(t, delegation.Timestamp, "Delegation %d should have timestamp", index)
	assert.NotEmpty(t, delegation.Amount, "Delegation %d should have amount", index)
	assert.NotEmpty(t, delegation.Delegator, "Delegation %d should have delegator", index)
	assert.NotEmpty(t, delegation.Level, "Delegation %d should have level", index)
}

// assertPaginationLinksPresent verifies Link header is present
func assertPaginationLinksPresent(t *testing.T, resp *http.Response) {
	t.Helper()

	linkHeader := resp.Header.Get("Link")
	assert.NotEmpty(t, linkHeader, "Should provide Link header when pagination is needed")
}

// assertPaginationLinksAbsent verifies Link header is absent
func assertPaginationLinksAbsent(t *testing.T, resp *http.Response) {
	t.Helper()

	linkHeader := resp.Header.Get("Link")
	assert.Empty(t, linkHeader, "Should omit Link header when all results fit on first page")
}

// assertContainsNextLink verifies Link header contains next link
func assertContainsNextLink(t *testing.T, resp *http.Response) {
	t.Helper()

	linkHeader := resp.Header.Get("Link")
	assert.Contains(t, linkHeader, `rel="next"`, "Should provide next link when more pages exist")
}

// assertMissingPrevLink verifies Link header does not contain prev link
func assertMissingPrevLink(t *testing.T, resp *http.Response) {
	t.Helper()

	linkHeader := resp.Header.Get("Link")
	assert.NotContains(t, linkHeader, `rel="prev"`, "Should not provide prev link on first page")
}

// assertContainsPrevLink verifies Link header contains prev link
func assertContainsPrevLink(t *testing.T, resp *http.Response) {
	t.Helper()

	linkHeader := resp.Header.Get("Link")
	assert.Contains(t, linkHeader, `rel="prev"`, "Should provide prev link when not on first page")
}

// assertCorrectPageNavigation verifies navigation links point to correct pages
func assertCorrectPageNavigation(t *testing.T, resp *http.Response, expectedPrevPage, expectedPerPage int) {
	t.Helper()

	linkHeader := resp.Header.Get("Link")
	assert.Contains(t, linkHeader, fmt.Sprintf("page=%d", expectedPrevPage), "Prev link should point to page %d", expectedPrevPage)
	assert.Contains(t, linkHeader, fmt.Sprintf("per_page=%d", expectedPerPage), "All links should preserve per_page parameter")
}

// assertPreservesQueryParameters verifies pagination links preserve query parameters
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

// parseJSONResponse parses HTTP response body as JSON into the specified type
func parseJSONResponse[T any](t *testing.T, resp *http.Response) T {
	t.Helper()

	defer resp.Body.Close()

	var result T
	err := json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err, "Response should be valid JSON")

	return result
}

// insertTestDelegations manually inserts a few test delegations for Link header omission test
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

// createTestServerWithIsolatedConnection creates a test server with its own connection pool
// to the provided database. Each test gets isolated connection resources but shares the read-only database.
func createTestServerWithIsolatedConnection(t *testing.T, dbConnString string) (*httptest.Server, func()) {
	t.Helper()

	// Each test gets its own connection pool to the shared read-only database
	storeConn, err := pgxdb.NewConnection(t.Context(), dbConnString)
	require.NoError(t, err)

	// Each test gets its own store
	store, storeCloser := pgxstore.New(storeConn)

	// Create server with isolated connection resources and logging (like production)
	mux := http.NewServeMux()
	tezosHandler := handler.NewTezosGetDelegations(store)
	tezosHandler.AddRoutes(mux)

	// Add logging middleware for SUT observability (like production)
	testCfg := testcfg.New()
	log := logger.NewFromConfig(logger.Config{
		LogLevel:         testCfg.LogLevel,
		LogHumanFriendly: testCfg.LogHumanFriendly,
	})
	loggedMux := logger.NewMiddleware(log)(mux)

	server := httptest.NewServer(loggedMux)

	// Return server and cleanup function for this test's resources
	cleanup := func() {
		server.Close()
		storeCloser() // Closes the connection pool for this test
	}

	return server, cleanup
}
