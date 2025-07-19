package tezos_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/screwyprof/delegator/web/tezos"
)

func TestParseYearFromUint64(t *testing.T) {
	t.Parallel()

	t.Run("when year is zero", func(t *testing.T) {
		t.Parallel()

		// Act
		year, err := tezos.ParseYearFromUint64(0)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, tezos.Year(0), year, "Zero should create year filter disabled")
		assert.Equal(t, uint64(0), year.Uint64(), "Should return zero value")
	})

	t.Run("when year is within valid range", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name         string
			input        uint64
			expectedYear tezos.Year
		}{
			{
				name:         "tezos launch year",
				input:        tezos.MinValidYear,
				expectedYear: tezos.Year(tezos.MinValidYear),
			},
			{
				name:         "current year",
				input:        uint64(time.Now().Year()),
				expectedYear: tezos.Year(time.Now().Year()),
			},
			{
				name:         "future year within buffer",
				input:        uint64(time.Now().Year()) + 5,
				expectedYear: tezos.Year(uint64(time.Now().Year()) + 5),
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				// Act
				year, err := tezos.ParseYearFromUint64(tc.input)

				// Assert
				require.NoError(t, err)
				assert.Equal(t, tc.expectedYear, year)
				assert.Equal(t, tc.input, year.Uint64())
			})
		}
	})

	t.Run("when year is below minimum valid year", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name  string
			input uint64
		}{
			{
				name:  "year before tezos launch",
				input: tezos.MinValidYear - 1,
			},
			{
				name:  "very old year",
				input: 1900,
			},
			{
				name:  "year 1",
				input: 1,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				// Act
				year, err := tezos.ParseYearFromUint64(tc.input)

				// Assert
				assert.Error(t, err)
				assert.ErrorIs(t, err, tezos.ErrYearOutOfRange)
				assert.Equal(t, tezos.Year(0), year, "Should return zero value on error")
			})
		}
	})

	t.Run("when year is above maximum valid year", func(t *testing.T) {
		t.Parallel()

		currentYear := uint64(time.Now().Year())
		maxValidYear := currentYear + tezos.MaxAllowedYearsInFuture

		testCases := []struct {
			name  string
			input uint64
		}{
			{
				name:  "one year beyond buffer",
				input: maxValidYear + 1,
			},
			{
				name:  "far future year",
				input: maxValidYear + 100,
			},
			{
				name:  "year 9999",
				input: 9999,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				// Act
				year, err := tezos.ParseYearFromUint64(tc.input)

				// Assert
				assert.Error(t, err)
				assert.ErrorIs(t, err, tezos.ErrYearOutOfRange)
				assert.Equal(t, tezos.Year(0), year, "Should return zero value on error")
			})
		}
	})

	t.Run("boundary values", func(t *testing.T) {
		t.Parallel()

		currentYear := uint64(time.Now().Year())
		maxValidYear := currentYear + tezos.MaxAllowedYearsInFuture

		testCases := []struct {
			name        string
			input       uint64
			shouldError bool
		}{
			{
				name:        "minimum valid year",
				input:       tezos.MinValidYear,
				shouldError: false,
			},
			{
				name:        "one below minimum",
				input:       tezos.MinValidYear - 1,
				shouldError: true,
			},
			{
				name:        "maximum valid year",
				input:       maxValidYear,
				shouldError: false,
			},
			{
				name:        "one above maximum",
				input:       maxValidYear + 1,
				shouldError: true,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				// Act
				year, err := tezos.ParseYearFromUint64(tc.input)

				// Assert
				if tc.shouldError {
					assert.Error(t, err)
					assert.ErrorIs(t, err, tezos.ErrYearOutOfRange)
					assert.Equal(t, tezos.Year(0), year)
				} else {
					require.NoError(t, err)
					assert.Equal(t, tezos.Year(tc.input), year)
					assert.Equal(t, tc.input, year.Uint64())
				}
			})
		}
	})
}

func TestYear_Uint64(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		year         tezos.Year
		expectedUint uint64
	}{
		{
			name:         "zero year",
			year:         tezos.Year(0),
			expectedUint: 0,
		},
		{
			name:         "tezos launch year",
			year:         tezos.Year(tezos.MinValidYear),
			expectedUint: tezos.MinValidYear,
		},
		{
			name:         "current year",
			year:         tezos.Year(2025),
			expectedUint: 2025,
		},
		{
			name:         "future year",
			year:         tezos.Year(2030),
			expectedUint: 2030,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Act
			result := tc.year.Uint64()

			// Assert
			assert.Equal(t, tc.expectedUint, result)
		})
	}
}
