package pgxstore

import (
	"fmt"

	"github.com/screwyprof/delegator/web/tezos"
)

// SQL queries
const (
	baseDelegationsQuery = "SELECT id, timestamp, amount, delegator, level FROM delegations"
)

// DelegationsQueryBuilder provides a domain-specific language for building delegation queries
type DelegationsQueryBuilder struct {
	sql  string
	args []any
}

// NewDelegationsQuery creates a new delegation query builder
func NewDelegationsQuery() *DelegationsQueryBuilder {
	return &DelegationsQueryBuilder{
		sql: baseDelegationsQuery,
	}
}

// ForCriteria applies the delegation criteria to the query in one fluent call
func (q *DelegationsQueryBuilder) ForCriteria(criteria tezos.DelegationsCriteria) *DelegationsQueryBuilder {
	return q.
		filterByYear(criteria.Year).
		orderByTimestampDesc().
		paginateWithDetection(criteria)
}

// filterByYear adds year filtering if the year is specified
func (q *DelegationsQueryBuilder) filterByYear(year tezos.Year) *DelegationsQueryBuilder {
	if year.Uint64() > 0 {
		q.addWhereCondition("year = $%d", year.Uint64())
	}
	return q
}

// orderByTimestampDesc adds timestamp ordering (most recent first)
func (q *DelegationsQueryBuilder) orderByTimestampDesc() *DelegationsQueryBuilder {
	q.sql += " ORDER BY timestamp DESC"
	return q
}

// paginateWithDetection adds pagination with "has more" detection using LIMIT n+1
func (q *DelegationsQueryBuilder) paginateWithDetection(criteria tezos.DelegationsCriteria) *DelegationsQueryBuilder {
	// Request one extra item to detect if there are more pages
	limit := criteria.ItemsPerPage() + 1
	offset := criteria.ItemsToSkip()

	q.addParameter("LIMIT $%d", limit)

	if offset > 0 {
		q.addParameter("OFFSET $%d", offset)
	}

	return q
}

// Build returns the final SQL query and arguments
func (q *DelegationsQueryBuilder) Build() (string, []any) {
	return q.sql, q.args
}

// Helper methods for building SQL

// addWhereCondition adds a WHERE condition, handling AND logic automatically
func (q *DelegationsQueryBuilder) addWhereCondition(sqlClause string, value any) {
	placeholder := q.nextPlaceholder()

	if q.hasWhereClause() {
		q.sql += " AND " + fmt.Sprintf(sqlClause, placeholder)
	} else {
		q.sql += " WHERE " + fmt.Sprintf(sqlClause, placeholder)
	}

	q.args = append(q.args, value)
}

// addParameter adds a SQL clause with a parameter
func (q *DelegationsQueryBuilder) addParameter(sqlClause string, value any) {
	placeholder := q.nextPlaceholder()
	q.sql += " " + fmt.Sprintf(sqlClause, placeholder)
	q.args = append(q.args, value)
}

// hasWhereClause checks if the query already has a WHERE clause
func (q *DelegationsQueryBuilder) hasWhereClause() bool {
	// Simple check - could be more sophisticated if needed
	return len(q.args) > 0
}

// nextPlaceholder returns the next PostgreSQL placeholder ($1, $2, etc.)
func (q *DelegationsQueryBuilder) nextPlaceholder() int {
	return len(q.args) + 1
}
