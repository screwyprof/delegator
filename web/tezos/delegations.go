package tezos

import (
	"context"
	"time"
)

// Delegation represents a delegation in the Tezos blockchain
type Delegation struct {
	ID        int64
	Timestamp time.Time
	Amount    int64
	Delegator string
	Level     int64
}

// DelegationsCriteria specifies criteria for querying delegations
type DelegationsCriteria struct {
	Year uint64 // Year filter (YYYY format). 0 means no year filtering
	Page uint64 // 1-based page number
	Size uint64 // Items per page
}

// DelegationsPage represents a page of delegation results with navigation metadata
type DelegationsPage struct {
	Delegations []Delegation
	HasMore     bool   // True if there are more pages after this one
	Number      uint64 // Current page number
	Size        uint64 // Page size
}

// Helper methods for pagination state
func (p *DelegationsPage) HasNext() bool     { return p.HasMore }
func (p *DelegationsPage) HasPrevious() bool { return p.Number > 1 }

// DelegationsFinder defines the interface for querying delegations
type DelegationsFinder interface {
	FindDelegations(ctx context.Context, criteria DelegationsCriteria) (*DelegationsPage, error)
}
