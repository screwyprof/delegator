////go:build acceptance

package web_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
)

// TestWebAPIAcceptanceBehavior tests end-to-end web API functionality
func TestWebAPIAcceptanceBehavior(t *testing.T) {
	t.Parallel()

	// Create ONE shared read-only test database for all subtests
	// Since we never modify data, this can be safely shared
	testCfg := testcfg.New()
	sharedTestDB := migratortest.CreateSeededTestDatabase(t, "../migrator/migrations", testCfg.SeedCheckpoint, testCfg.SeedChunkSize, testCfg.SeedTimeout)
	t.Cleanup(func() {
		sharedTestDB.Close()
	})

	// Get the connection string to the shared database
	dbConnString := sharedTestDB.Config().ConnString()

	t.Run("it returns delegations with default pagination and ordering", func(t *testing.T) {
		t.Parallel()

		// Each test creates its own connection pool to the shared read-only database
		server, cleanup := createTestServerWithIsolatedConnection(t, dbConnString)
		t.Cleanup(cleanup)
		client := &http.Client{}

		// Arrange
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/xtz/delegations", nil)
		require.NoError(t, err, "Should create HTTP request")

		// Act
		resp, err := client.Do(req)

		// Assert
		require.NoError(t, err, "HTTP request should succeed")
		assert.Equal(t, http.StatusOK, resp.StatusCode, "Should return HTTP 200 OK")

		defer resp.Body.Close()

		var response api.DelegationsResponse
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err, "Response should be valid JSON")

		// Note: We expect exactly 50 because the seeded database should have more than 50 delegations
		assert.Equal(t, 50, len(response.Data), "Should return exactly 50 delegations (default pagination limit)")

		// Validate ordering: most recent first (descending timestamps)
		if len(response.Data) > 1 {
			for i := 0; i < len(response.Data)-1; i++ {
				current := response.Data[i].Timestamp
				next := response.Data[i+1].Timestamp
				assert.GreaterOrEqual(t, current, next,
					"Delegations should be ordered most recent first (index %d: %s should be >= %s)",
					i, current, next)
			}
			t.Logf("✅ Ordering verified: most recent first")
		}

		// Validate response format matches TASK.md specification
		for i, delegation := range response.Data {
			assert.NotEmpty(t, delegation.Timestamp, "Delegation %d should have timestamp", i)
			assert.NotEmpty(t, delegation.Amount, "Delegation %d should have amount", i)
			assert.NotEmpty(t, delegation.Delegator, "Delegation %d should have delegator", i)
			assert.NotEmpty(t, delegation.Level, "Delegation %d should have level", i)
		}

		t.Logf("✅ Default pagination test completed successfully")
	})

	t.Run("it filters delegations by year parameter", func(t *testing.T) {
		t.Parallel()

		// Each test creates its own connection pool to the shared read-only database
		server, cleanup := createTestServerWithIsolatedConnection(t, dbConnString)
		t.Cleanup(cleanup)
		client := &http.Client{}

		// Arrange - Use 2025 since our seeded data is recent
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/xtz/delegations?year=2025", nil)
		require.NoError(t, err, "Should create HTTP request")

		// Act
		resp, err := client.Do(req)

		// Assert
		require.NoError(t, err, "HTTP request should succeed")
		assert.Equal(t, http.StatusOK, resp.StatusCode, "Should return HTTP 200 OK")

		defer resp.Body.Close()

		var response api.DelegationsResponse
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err, "Response should be valid JSON")

		// Validate that we got some results (seeded data should have 2025 delegations)
		assert.Greater(t, len(response.Data), 0, "Should return some delegations for year 2025")
		t.Logf("📋 Year filtering response: %d delegations for 2025", len(response.Data))

		// Validate that ALL returned delegations are from 2025
		for i, delegation := range response.Data {
			timestamp, err := time.Parse(time.RFC3339, delegation.Timestamp)
			require.NoError(t, err, "Should parse delegation timestamp")

			year := timestamp.Year()
			assert.Equal(t, 2025, year, "Delegation %d should be from year 2025, got %d", i, year)
		}

		t.Logf("✅ Year filtering test completed successfully")
	})

	t.Run("it provides GitHub-style pagination Link headers", func(t *testing.T) {
		t.Parallel()

		t.Run("it omits Link header when results fit on first page", func(t *testing.T) {
			t.Parallel()

			// Create clean database with only schema (no seeded data)
			cleanTestDB := migratortest.CreateScraperTestDatabase(t, "../migrator/migrations", 0)
			t.Cleanup(func() {
				cleanTestDB.Close()
			})

			// Manually insert just 2 test delegations (fits in one page)
			insertTestDelegations(t, cleanTestDB)

			// Create server with clean database
			server, cleanup := createTestServerWithIsolatedConnection(t, cleanTestDB.Config().ConnString())
			t.Cleanup(cleanup)
			client := &http.Client{}

			// Arrange - request with default page size (50), only 2 records exist
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/xtz/delegations", nil)
			require.NoError(t, err, "Should create HTTP request")

			// Act
			resp, err := client.Do(req)

			// Assert
			require.NoError(t, err, "HTTP request should succeed")
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode, "Should return OK")

			// Parse response to verify we got results that fit on one page
			var response api.DelegationsResponse
			err = json.NewDecoder(resp.Body).Decode(&response)
			require.NoError(t, err, "Should parse JSON response")
			assert.Equal(t, 2, len(response.Data), "Should return exactly 2 manually inserted delegations")

			// When results fit on first page, Link header should be omitted
			linkHeader := resp.Header.Get("Link")
			assert.Empty(t, linkHeader, "Should omit Link header when all results fit on first page")
		})

		t.Run("it provides next link on first page when more pages exist", func(t *testing.T) {
			t.Parallel()

			// Arrange
			server, cleanup := createTestServerWithIsolatedConnection(t, dbConnString)
			t.Cleanup(cleanup)
			client := &http.Client{}

			// Request small page size to ensure multiple pages
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/xtz/delegations?per_page=10", nil)
			require.NoError(t, err, "Should create HTTP request")

			// Act
			resp, err := client.Do(req)

			// Assert
			require.NoError(t, err, "HTTP request should succeed")
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode, "Should return OK")

			linkHeader := resp.Header.Get("Link")
			assert.NotEmpty(t, linkHeader, "Should provide Link header when more pages exist")
			assert.Contains(t, linkHeader, `rel="next"`, "Should provide next link on first page")
			assert.NotContains(t, linkHeader, `rel="prev"`, "Should not provide prev link on first page")
		})

		t.Run("it provides navigation links on middle pages", func(t *testing.T) {
			t.Parallel()

			// Arrange
			server, cleanup := createTestServerWithIsolatedConnection(t, dbConnString)
			t.Cleanup(cleanup)
			client := &http.Client{}

			// Request page 2 with small page size to ensure we're in the middle
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/xtz/delegations?page=2&per_page=10", nil)
			require.NoError(t, err, "Should create HTTP request")

			// Act
			resp, err := client.Do(req)

			// Assert
			require.NoError(t, err, "HTTP request should succeed")
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode, "Should return OK")

			linkHeader := resp.Header.Get("Link")
			assert.NotEmpty(t, linkHeader, "Should provide Link header on middle pages")
			assert.Contains(t, linkHeader, `rel="prev"`, "Should provide prev link when not on first page")

			// Check that URLs are correctly formed
			assert.Contains(t, linkHeader, "page=1", "Prev link should point to page 1")
			assert.Contains(t, linkHeader, "per_page=10", "All links should preserve per_page parameter")
		})

		t.Run("it preserves query parameters in pagination links", func(t *testing.T) {
			t.Parallel()

			// Arrange
			server, cleanup := createTestServerWithIsolatedConnection(t, dbConnString)
			t.Cleanup(cleanup)
			client := &http.Client{}

			// Request page 2 with year filter and small page size
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/xtz/delegations?page=2&per_page=5&year=2025", nil)
			require.NoError(t, err, "Should create HTTP request")

			// Act
			resp, err := client.Do(req)

			// Assert
			require.NoError(t, err, "HTTP request should succeed")
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode, "Should return OK")

			linkHeader := resp.Header.Get("Link")
			assert.NotEmpty(t, linkHeader, "Should provide Link header on middle pages with parameters")
			// All navigation links should preserve the year filter
			assert.Contains(t, linkHeader, "year=2025", "Should preserve year parameter in navigation links")
			assert.Contains(t, linkHeader, "per_page=5", "Should preserve per_page parameter in navigation links")
		})
	})
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
