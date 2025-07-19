package tezos_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/screwyprof/delegator/web/tezos"
)

func TestNewDelegationsCriteria(t *testing.T) {
	t.Parallel()

	t.Run("when all parameters are valid", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name        string
			year        uint64
			page        uint64
			perPage     uint64
			expectedErr error
		}{
			{
				name:        "zero values use defaults",
				year:        0,
				page:        0,
				perPage:     0,
				expectedErr: nil,
			},
			{
				name:        "valid tezos launch year",
				year:        2018,
				page:        1,
				perPage:     25,
				expectedErr: nil,
			},
			{
				name:        "current year with high page number",
				year:        2025,
				page:        999,
				perPage:     100,
				expectedErr: nil,
			},
			{
				name:        "no year filter with pagination",
				year:        0,
				page:        5,
				perPage:     10,
				expectedErr: nil,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				// Act
				criteria, err := tezos.NewDelegationsCriteria(tc.year, tc.page, tc.perPage)

				// Assert
				if tc.expectedErr != nil {
					assert.Error(t, err)
					assert.ErrorIs(t, err, tc.expectedErr)
				} else {
					require.NoError(t, err)
					assert.Equal(t, tc.year, criteria.Year.Uint64())

					// Verify default handling
					expectedPage := tc.page
					if expectedPage == 0 {
						expectedPage = tezos.DefaultPage
					}
					assert.Equal(t, expectedPage, criteria.Page.Uint64())

					expectedPerPage := tc.perPage
					if expectedPerPage == 0 {
						expectedPerPage = tezos.DefaultPerPage
					}
					assert.Equal(t, expectedPerPage, criteria.Size.Uint64())
				}
			})
		}
	})

	t.Run("when year parameter is invalid", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name    string
			year    uint64
			page    uint64
			perPage uint64
		}{
			{
				name:    "year before tezos launch",
				year:    2017,
				page:    1,
				perPage: 50,
			},
			{
				name:    "year too far in future",
				year:    9999,
				page:    1,
				perPage: 50,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				// Act
				criteria, err := tezos.NewDelegationsCriteria(tc.year, tc.page, tc.perPage)

				// Assert
				assert.Error(t, err)
				assert.ErrorIs(t, err, tezos.ErrInvalidYear)
				assert.Equal(t, tezos.DelegationsCriteria{}, criteria, "Should return zero value on error")
			})
		}
	})

	t.Run("when per_page parameter is invalid", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name    string
			year    uint64
			page    uint64
			perPage uint64
		}{
			{
				name:    "per_page exceeds maximum",
				year:    2025,
				page:    1,
				perPage: tezos.MaxPerPage + 1,
			},
			{
				name:    "per_page way too large",
				year:    0,
				page:    1,
				perPage: 999,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				// Act
				criteria, err := tezos.NewDelegationsCriteria(tc.year, tc.page, tc.perPage)

				// Assert
				assert.Error(t, err)
				assert.ErrorIs(t, err, tezos.ErrInvalidPerPage)
				assert.Equal(t, tezos.DelegationsCriteria{}, criteria, "Should return zero value on error")
			})
		}
	})

	t.Run("error precedence", func(t *testing.T) {
		t.Parallel()

		// When multiple parameters are invalid, should return first error encountered
		// (year is validated first, then page, then perPage)

		// Act - invalid year AND invalid perPage
		criteria, err := tezos.NewDelegationsCriteria(1999, 1, 999)

		// Assert
		assert.Error(t, err)
		assert.ErrorIs(t, err, tezos.ErrInvalidYear, "Should return year error first")
		assert.Equal(t, tezos.DelegationsCriteria{}, criteria)
	})
}

func TestDelegationsCriteria_ItemsPerPage(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		criteria tezos.DelegationsCriteria
		expected uint64
	}{
		{
			name: "default per_page",
			criteria: tezos.DelegationsCriteria{
				Size: tezos.PerPage(tezos.DefaultPerPage),
			},
			expected: tezos.DefaultPerPage,
		},
		{
			name: "small per_page",
			criteria: tezos.DelegationsCriteria{
				Size: tezos.PerPage(5),
			},
			expected: 5,
		},
		{
			name: "maximum per_page",
			criteria: tezos.DelegationsCriteria{
				Size: tezos.PerPage(tezos.MaxPerPage),
			},
			expected: tezos.MaxPerPage,
		},
		{
			name: "minimum per_page",
			criteria: tezos.DelegationsCriteria{
				Size: tezos.PerPage(1),
			},
			expected: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Act
			result := tc.criteria.ItemsPerPage()

			// Assert
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestDelegationsCriteria_ItemsToSkip(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		criteria tezos.DelegationsCriteria
		expected uint64
	}{
		{
			name: "first page should skip zero items",
			criteria: tezos.DelegationsCriteria{
				Page: tezos.Page(1),
				Size: tezos.PerPage(50),
			},
			expected: 0,
		},
		{
			name: "second page should skip first page items",
			criteria: tezos.DelegationsCriteria{
				Page: tezos.Page(2),
				Size: tezos.PerPage(50),
			},
			expected: 50,
		},
		{
			name: "third page with small page size",
			criteria: tezos.DelegationsCriteria{
				Page: tezos.Page(3),
				Size: tezos.PerPage(10),
			},
			expected: 20,
		},
		{
			name: "high page number",
			criteria: tezos.DelegationsCriteria{
				Page: tezos.Page(10),
				Size: tezos.PerPage(25),
			},
			expected: 225, // (10-1) * 25
		},
		{
			name: "page size of 1",
			criteria: tezos.DelegationsCriteria{
				Page: tezos.Page(5),
				Size: tezos.PerPage(1),
			},
			expected: 4, // (5-1) * 1
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Act
			result := tc.criteria.ItemsToSkip()

			// Assert
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestDelegationsCriteria_Integration(t *testing.T) {
	t.Parallel()

	t.Run("complete criteria construction and usage", func(t *testing.T) {
		t.Parallel()

		// Arrange
		year := uint64(2025)
		page := uint64(3)
		perPage := uint64(25)

		// Act
		criteria, err := tezos.NewDelegationsCriteria(year, page, perPage)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, year, criteria.Year.Uint64())
		assert.Equal(t, page, criteria.Page.Uint64())
		assert.Equal(t, perPage, criteria.Size.Uint64())

		// Verify calculations work correctly
		assert.Equal(t, perPage, criteria.ItemsPerPage())
		assert.Equal(t, uint64(50), criteria.ItemsToSkip()) // (3-1) * 25
	})

	t.Run("default values integration", func(t *testing.T) {
		t.Parallel()

		// Act - use all defaults
		criteria, err := tezos.NewDelegationsCriteria(0, 0, 0)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, uint64(0), criteria.Year.Uint64(), "Year 0 means no filtering")
		assert.Equal(t, uint64(tezos.DefaultPage), criteria.Page.Uint64())
		assert.Equal(t, uint64(tezos.DefaultPerPage), criteria.Size.Uint64())

		// Verify calculations with defaults
		assert.Equal(t, uint64(tezos.DefaultPerPage), criteria.ItemsPerPage())
		assert.Equal(t, uint64(0), criteria.ItemsToSkip(), "First page skips 0 items")
	})
}
