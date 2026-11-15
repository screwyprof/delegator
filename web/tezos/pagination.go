package tezos

import (
	"errors"
	"fmt"
	"time"
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

// Cursor is a delegation's position in newest-first order (timestamp DESC, id DESC).
// id breaks timestamp ties (many delegations share a block timestamp), so every delegation
// has a distinct position, and paging relative to one is unaffected by rows added elsewhere.
type Cursor struct {
	Timestamp time.Time
	ID        int64
}

// CursorOf returns the position of d
func CursorOf(d Delegation) Cursor {
	return Cursor{Timestamp: d.Timestamp, ID: d.ID}
}

// DelegationsPage represents a page of delegation results with navigation metadata
type DelegationsPage struct {
	Delegations []Delegation // Newest first
	Size        PerPage      // Page size

	hasNext     bool
	hasPrevious bool
	isFirst     bool
}

// NewDelegationsPage builds the page for criteria. delegations must already be trimmed to
// the page size and in newest-first order; overflowed reports whether the query found one
// more row beyond the page in the direction it read.
func NewDelegationsPage(criteria DelegationsCriteria, delegations []Delegation, overflowed bool) *DelegationsPage {
	page := &DelegationsPage{
		Delegations: delegations,
		Size:        criteria.Size,
		isFirst:     criteria.After == nil && criteria.Before == nil && criteria.Page <= 1,
	}

	if criteria.SeeksBackward() {
		// Read newer rows going back from a Before cursor: the row that cursor came from
		// (and everything older) is the next page.
		page.hasPrevious = overflowed
		page.hasNext = true
	} else {
		page.hasNext = overflowed
		page.hasPrevious = criteria.After != nil || criteria.Page > 1
	}

	return page
}

// HasFirst reports whether to link back to the first page: any page but the first, even an
// empty one (e.g. a page number past the end), since that link needs no cursor
func (p *DelegationsPage) HasFirst() bool { return !p.isFirst }

// HasNext reports whether older delegations follow this page
func (p *DelegationsPage) HasNext() bool { return p.hasNext && len(p.Delegations) > 0 }

// HasPrevious reports whether newer delegations precede this page
func (p *DelegationsPage) HasPrevious() bool { return p.hasPrevious && len(p.Delegations) > 0 }

// NextCursor is where the next (older) page continues from; only meaningful when HasNext
func (p *DelegationsPage) NextCursor() Cursor {
	return CursorOf(p.Delegations[len(p.Delegations)-1])
}

// PreviousCursor is where the previous (newer) page reads back from; only meaningful when HasPrevious
func (p *DelegationsPage) PreviousCursor() Cursor {
	return CursorOf(p.Delegations[0])
}
