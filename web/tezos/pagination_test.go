package tezos_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/screwyprof/delegator/web/tezos"
)

func TestParsePageFromUint64(t *testing.T) {
	t.Parallel()

	t.Run("when page is zero", func(t *testing.T) {
		t.Parallel()

		// Act
		page := tezos.ParsePageFromUint64(0)

		// Assert
		assert.Equal(t, tezos.Page(tezos.DefaultPage), page, "Zero should default to first page")
		assert.Equal(t, uint64(tezos.DefaultPage), page.Uint64())
	})

	t.Run("when page is positive", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name         string
			input        uint64
			expectedPage tezos.Page
		}{
			{
				name:         "first page",
				input:        1,
				expectedPage: tezos.Page(1),
			},
			{
				name:         "second page",
				input:        2,
				expectedPage: tezos.Page(2),
			},
			{
				name:         "high page number",
				input:        999,
				expectedPage: tezos.Page(999),
			},
			{
				name:         "maximum uint64",
				input:        ^uint64(0),
				expectedPage: tezos.Page(^uint64(0)),
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				// Act
				page := tezos.ParsePageFromUint64(tc.input)

				// Assert
				assert.Equal(t, tc.expectedPage, page)
				assert.Equal(t, tc.input, page.Uint64())
			})
		}
	})
}

func TestParsePerPageFromUint64(t *testing.T) {
	t.Parallel()

	t.Run("when per_page is zero", func(t *testing.T) {
		t.Parallel()

		// Act
		perPage, err := tezos.ParsePerPageFromUint64(0)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, tezos.PerPage(tezos.DefaultPerPage), perPage, "Zero should default to %d", tezos.DefaultPerPage)
		assert.Equal(t, uint64(tezos.DefaultPerPage), perPage.Uint64())
	})

	t.Run("when per_page is within valid range", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name            string
			input           uint64
			expectedPerPage tezos.PerPage
		}{
			{
				name:            "minimum valid per_page",
				input:           1,
				expectedPerPage: tezos.PerPage(1),
			},
			{
				name:            "small per_page",
				input:           5,
				expectedPerPage: tezos.PerPage(5),
			},
			{
				name:            "default per_page",
				input:           tezos.DefaultPerPage,
				expectedPerPage: tezos.PerPage(tezos.DefaultPerPage),
			},
			{
				name:            "large per_page",
				input:           75,
				expectedPerPage: tezos.PerPage(75),
			},
			{
				name:            "maximum valid per_page",
				input:           tezos.MaxPerPage,
				expectedPerPage: tezos.PerPage(tezos.MaxPerPage),
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				// Act
				perPage, err := tezos.ParsePerPageFromUint64(tc.input)

				// Assert
				require.NoError(t, err)
				assert.Equal(t, tc.expectedPerPage, perPage)
				assert.Equal(t, tc.input, perPage.Uint64())
			})
		}
	})

	t.Run("when per_page exceeds maximum", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name  string
			input uint64
		}{
			{
				name:  "one above maximum",
				input: tezos.MaxPerPage + 1,
			},
			{
				name:  "large value",
				input: 500,
			},
			{
				name:  "very large value",
				input: 999999,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				// Act
				perPage, err := tezos.ParsePerPageFromUint64(tc.input)

				// Assert
				assert.Error(t, err)
				assert.ErrorIs(t, err, tezos.ErrPerPageTooLarge)
				assert.Equal(t, tezos.PerPage(0), perPage, "Should return zero value on error")
			})
		}
	})

	t.Run("boundary values", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name        string
			input       uint64
			shouldError bool
		}{
			{
				name:        "zero (should default)",
				input:       0,
				shouldError: false,
			},
			{
				name:        "one (minimum valid)",
				input:       1,
				shouldError: false,
			},
			{
				name:        "maximum valid",
				input:       tezos.MaxPerPage,
				shouldError: false,
			},
			{
				name:        "one above maximum",
				input:       tezos.MaxPerPage + 1,
				shouldError: true,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				// Act
				perPage, err := tezos.ParsePerPageFromUint64(tc.input)

				// Assert
				if tc.shouldError {
					assert.Error(t, err)
					assert.ErrorIs(t, err, tezos.ErrPerPageTooLarge)
					assert.Equal(t, tezos.PerPage(0), perPage)
				} else {
					require.NoError(t, err)
					if tc.input == 0 {
						assert.Equal(t, tezos.PerPage(tezos.DefaultPerPage), perPage)
						assert.Equal(t, uint64(tezos.DefaultPerPage), perPage.Uint64())
					} else {
						assert.Equal(t, tezos.PerPage(tc.input), perPage)
						assert.Equal(t, tc.input, perPage.Uint64())
					}
				}
			})
		}
	})
}

func TestPage_Uint64(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		page         tezos.Page
		expectedUint uint64
	}{
		{
			name:         "first page",
			page:         tezos.Page(1),
			expectedUint: 1,
		},
		{
			name:         "second page",
			page:         tezos.Page(2),
			expectedUint: 2,
		},
		{
			name:         "high page number",
			page:         tezos.Page(999),
			expectedUint: 999,
		},
		{
			name:         "zero page",
			page:         tezos.Page(0),
			expectedUint: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Act
			result := tc.page.Uint64()

			// Assert
			assert.Equal(t, tc.expectedUint, result)
		})
	}
}

func TestPerPage_Uint64(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		perPage      tezos.PerPage
		expectedUint uint64
	}{
		{
			name:         "minimum per_page",
			perPage:      tezos.PerPage(1),
			expectedUint: 1,
		},
		{
			name:         "default per_page",
			perPage:      tezos.PerPage(tezos.DefaultPerPage),
			expectedUint: tezos.DefaultPerPage,
		},
		{
			name:         "maximum per_page",
			perPage:      tezos.PerPage(tezos.MaxPerPage),
			expectedUint: tezos.MaxPerPage,
		},
		{
			name:         "zero per_page",
			perPage:      tezos.PerPage(0),
			expectedUint: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Act
			result := tc.perPage.Uint64()

			// Assert
			assert.Equal(t, tc.expectedUint, result)
		})
	}
}

func TestDelegationsPage_HasNext(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		hasMore     bool
		expectedVal bool
	}{
		{
			name:        "has more pages",
			hasMore:     true,
			expectedVal: true,
		},
		{
			name:        "no more pages",
			hasMore:     false,
			expectedVal: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			page := &tezos.DelegationsPage{HasMore: tc.hasMore}

			// Act
			result := page.HasNext()

			// Assert
			assert.Equal(t, tc.expectedVal, result)
		})
	}
}

func TestDelegationsPage_HasPrevious(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		pageNumber  tezos.Page
		expectedVal bool
	}{
		{
			name:        "first page",
			pageNumber:  tezos.Page(1),
			expectedVal: false,
		},
		{
			name:        "second page",
			pageNumber:  tezos.Page(2),
			expectedVal: true,
		},
		{
			name:        "high page number",
			pageNumber:  tezos.Page(10),
			expectedVal: true,
		},
		{
			name:        "zero page (edge case)",
			pageNumber:  tezos.Page(0),
			expectedVal: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			page := &tezos.DelegationsPage{Number: tc.pageNumber}

			// Act
			result := page.HasPrevious()

			// Assert
			assert.Equal(t, tc.expectedVal, result)
		})
	}
}
