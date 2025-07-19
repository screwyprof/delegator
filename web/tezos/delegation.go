package tezos

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Sentinel errors for delegation criteria construction
var (
	ErrInvalidYear    = errors.New("invalid year")
	ErrInvalidPage    = errors.New("invalid page")
	ErrInvalidPerPage = errors.New("invalid per_page")
)

// DelegationsFinder defines the interface for querying delegations
type DelegationsFinder interface {
	FindDelegations(ctx context.Context, criteria DelegationsCriteria) (*DelegationsPage, error)
}

// Delegation represents a delegation in the Tezos blockchain
type Delegation struct {
	ID        int64
	Timestamp time.Time
	Amount    int64
	Delegator string
	Level     int64
}

// DelegationsCriteria specifies criteria for querying delegations using domain Value Objects
type DelegationsCriteria struct {
	Year Year    // Year filter (YYYY format). 0 means no year filtering
	Page Page    // 1-based page number
	Size PerPage // Items per page
}

// ItemsPerPage returns the number of items requested per page
func (c DelegationsCriteria) ItemsPerPage() uint64 {
	return c.Size.Uint64()
}

// ItemsToSkip returns the number of items to skip for pagination
func (c DelegationsCriteria) ItemsToSkip() uint64 {
	return (c.Page.Uint64() - 1) * c.Size.Uint64()
}

// NewDelegationsCriteria creates DelegationsCriteria from uint64 values with validation
func NewDelegationsCriteria(year, page, perPage uint64) (DelegationsCriteria, error) {
	y, err := ParseYearFromUint64(year)
	if err != nil {
		return DelegationsCriteria{}, fmt.Errorf("%w: %w", ErrInvalidYear, err)
	}

	p, err := ParsePageFromUint64(page)
	if err != nil {
		return DelegationsCriteria{}, fmt.Errorf("%w: %w", ErrInvalidPage, err)
	}

	pp, err := ParsePerPageFromUint64(perPage)
	if err != nil {
		return DelegationsCriteria{}, fmt.Errorf("%w: %w", ErrInvalidPerPage, err)
	}

	return DelegationsCriteria{
		Year: y,
		Page: p,
		Size: pp,
	}, nil
}
