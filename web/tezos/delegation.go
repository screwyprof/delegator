// Package tezos is the web API's domain: delegations, the criteria for querying them, and
// cursor-based pages of results.
package tezos

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Sentinel errors for delegation criteria construction.
var (
	ErrInvalidYear    = errors.New("invalid year")
	ErrInvalidPage    = errors.New("invalid page")
	ErrInvalidPerPage = errors.New("invalid per_page")

	ErrConflictingPagination = errors.New("conflicting pagination parameters")
)

// DelegationsFinder defines the interface for querying delegations.
type DelegationsFinder interface {
	FindDelegations(ctx context.Context, criteria DelegationsCriteria) (*DelegationsPage, error)
}

// Delegation represents a delegation in the Tezos blockchain.
type Delegation struct {
	ID        int64
	Timestamp time.Time
	Amount    int64
	Delegator string
	Level     int64
}

// DelegationsCriteria specifies criteria for querying delegations using domain Value Objects.
type DelegationsCriteria struct {
	Year Year    // Year filter (YYYY format). 0 means no year filtering
	Page Page    // 1-based page number; only used when neither cursor is set
	Size PerPage // Items per page

	After  *Cursor // Page of delegations strictly older than this position
	Before *Cursor // Page of delegations strictly newer than this position
}

// CriteriaOption sets an optional part of DelegationsCriteria.
type CriteriaOption func(*DelegationsCriteria)

// After pages from just past c, toward older delegations.
func After(c Cursor) CriteriaOption {
	return func(criteria *DelegationsCriteria) { criteria.After = &c }
}

// Before pages from just ahead of c, toward newer delegations.
func Before(c Cursor) CriteriaOption {
	return func(criteria *DelegationsCriteria) { criteria.Before = &c }
}

// ItemsPerPage returns the number of items requested per page.
func (c DelegationsCriteria) ItemsPerPage() uint64 {
	return c.Size.Uint64()
}

// ItemsToSkip returns the number of items to skip for page-number pagination (0 with a cursor).
func (c DelegationsCriteria) ItemsToSkip() uint64 {
	if c.After != nil || c.Before != nil {
		return 0
	}

	return (c.Page.Uint64() - 1) * c.Size.Uint64()
}

// SeeksBackward reports whether the page is read toward newer delegations (a Before cursor).
func (c DelegationsCriteria) SeeksBackward() bool {
	return c.Before != nil
}

// NewDelegationsCriteria creates DelegationsCriteria from uint64 values with validation.
// page is 0 when not given; it can't be combined with a cursor option, nor can the two cursors.
func NewDelegationsCriteria(year, page, perPage uint64, opts ...CriteriaOption) (DelegationsCriteria, error) {
	y, err := ParseYearFromUint64(year)
	if err != nil {
		return DelegationsCriteria{}, fmt.Errorf("%w: %w", ErrInvalidYear, err)
	}

	p := ParsePageFromUint64(page) // 0 means the first page; the offset check below bounds the rest

	pp, err := ParsePerPageFromUint64(perPage)
	if err != nil {
		return DelegationsCriteria{}, fmt.Errorf("%w: %w", ErrInvalidPerPage, err)
	}

	// Serve page numbers only up to MaxPageOffset skipped rows: OFFSET cost grows with depth, and
	// cursor links reach any depth at the same cost. Dividing rather than multiplying keeps the
	// check itself from overflowing, which in uint64 would wrap around to a different page.
	if p.Uint64()-1 > MaxPageOffset/pp.Uint64() {
		return DelegationsCriteria{}, fmt.Errorf(
			"%w: page %d skips more than %d delegations at per_page %d; follow the rel=\"next\" links to go deeper",
			ErrInvalidPage, p, MaxPageOffset, pp)
	}

	criteria := DelegationsCriteria{
		Year: y,
		Page: p,
		Size: pp,
	}
	for _, opt := range opts {
		opt(&criteria)
	}

	if criteria.After != nil && criteria.Before != nil {
		return DelegationsCriteria{}, fmt.Errorf(
			"%w: after and before are mutually exclusive",
			ErrConflictingPagination,
		)
	}

	if page != 0 && (criteria.After != nil || criteria.Before != nil) {
		return DelegationsCriteria{}, fmt.Errorf("%w: page can't be combined with a cursor", ErrConflictingPagination)
	}

	return criteria, nil
}
