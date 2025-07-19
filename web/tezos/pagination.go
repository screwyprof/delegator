package tezos

import (
	"errors"
	"fmt"
)

// Default pagination values
const (
	DefaultPage    = 1   // Default to first page
	DefaultPerPage = 50  // Default pagination size
	MaxPerPage     = 100 // Maximum items per page
)

// Page represents a page number for pagination
type Page uint64

// PerPage represents items per page for pagination
type PerPage uint64

// Pagination validation errors
var (
	ErrPerPageNotPositive = errors.New("per_page must be positive")
	ErrPerPageTooLarge    = errors.New("per_page exceeds maximum limit")
)

// ParsePageFromUint64 creates a Page from uint64 with default handling
func ParsePageFromUint64(page uint64) Page {
	// Zero means use default page
	if page == 0 {
		return Page(DefaultPage)
	}

	return Page(page)
}

// ParsePerPageFromUint64 creates a PerPage from uint64 with domain validation
func ParsePerPageFromUint64(perPage uint64) (PerPage, error) {
	// Zero means use default per_page
	if perPage == 0 {
		return PerPage(DefaultPerPage), nil
	}

	if perPage > MaxPerPage {
		return 0, fmt.Errorf("%w: must be between 1 and %d", ErrPerPageTooLarge, MaxPerPage)
	}

	return PerPage(perPage), nil
}

// Uint64 returns the underlying uint64 value
func (p Page) Uint64() uint64 {
	return uint64(p)
}

// Uint64 returns the underlying uint64 value
func (pp PerPage) Uint64() uint64 {
	return uint64(pp)
}

// DelegationsPage represents a page of delegation results with navigation metadata
type DelegationsPage struct {
	Delegations []Delegation
	HasMore     bool    // True if there are more pages after this one
	Number      Page    // Current page number
	Size        PerPage // Page size
}

// Helper methods for pagination state
func (p *DelegationsPage) HasNext() bool     { return p.HasMore }
func (p *DelegationsPage) HasPrevious() bool { return p.Number > 1 }
