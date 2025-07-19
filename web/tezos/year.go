package tezos

import (
	"errors"
	"time"
)

// Year validation constants
const (
	MinValidYear            = 2018 // Year when Tezos mainnet launched
	MaxAllowedYearsInFuture = 10   // Allow this many years into the future
)

// Year represents a year value for delegation filtering
type Year uint64

// Year validation errors
var (
	ErrYearOutOfRange = errors.New("year out of valid range")
)

// ParseYearFromUint64 creates a Year from uint64 with domain validation
func ParseYearFromUint64(year uint64) (Year, error) {
	// Zero means no year filter (use default)
	if year == 0 {
		return Year(0), nil
	}

	// Must be in reasonable range for Tezos launch year + some future buffer
	currentYear := uint64(time.Now().Year())
	maxValidYear := currentYear + MaxAllowedYearsInFuture

	if year < MinValidYear || year > maxValidYear {
		return 0, ErrYearOutOfRange
	}

	return Year(year), nil
}

// Uint64 returns the underlying uint64 value
func (y Year) Uint64() uint64 {
	return uint64(y)
}
