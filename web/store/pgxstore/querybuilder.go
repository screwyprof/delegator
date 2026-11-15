package pgxstore

import (
	"fmt"
	"strings"

	"github.com/screwyprof/delegator/web/tezos"
)

// SQL queries.
const (
	baseDelegationsQuery = "SELECT id, timestamp, amount, delegator, level FROM delegations"
)

// DelegationsQueryBuilder provides a domain-specific language for building delegation queries.
type DelegationsQueryBuilder struct {
	sql   string
	where []string
	args  []any
}

// NewDelegationsQuery creates a new delegation query builder.
func NewDelegationsQuery() *DelegationsQueryBuilder {
	return &DelegationsQueryBuilder{
		sql: baseDelegationsQuery,
	}
}

// ForCriteria applies the delegation criteria to the query in one fluent call.
func (q *DelegationsQueryBuilder) ForCriteria(criteria tezos.DelegationsCriteria) *DelegationsQueryBuilder {
	return q.
		filterByYear(criteria.Year).
		seekFromCursor(criteria).
		orderByPosition(criteria).
		paginateWithDetection(criteria)
}

// filterByYear adds year filtering if the year is specified.
func (q *DelegationsQueryBuilder) filterByYear(year tezos.Year) *DelegationsQueryBuilder {
	if year.Uint64() > 0 {
		q.addWhereCondition("year = $%d", year.Uint64())
	}

	return q
}

// seekFromCursor restricts the query to delegations strictly past the cursor's position.
// The (timestamp, id) row comparison matches the ordering exactly, so it's served by the
// (…, timestamp DESC, id DESC) indexes instead of scanning past skipped rows like OFFSET.
func (q *DelegationsQueryBuilder) seekFromCursor(criteria tezos.DelegationsCriteria) *DelegationsQueryBuilder {
	switch {
	case criteria.After != nil:
		q.addWhereCondition("(timestamp, id) < ($%d, $%d)", criteria.After.Timestamp, criteria.After.ID)
	case criteria.Before != nil:
		q.addWhereCondition("(timestamp, id) > ($%d, $%d)", criteria.Before.Timestamp, criteria.Before.ID)
	}

	return q
}

// orderByPosition orders most recent first, with id breaking timestamp ties so the order
// (and therefore every page boundary) is deterministic. Seeking backward from a Before
// cursor reads the opposite way so LIMIT keeps the rows nearest the cursor; the store
// reverses them back to most recent first.
func (q *DelegationsQueryBuilder) orderByPosition(criteria tezos.DelegationsCriteria) *DelegationsQueryBuilder {
	if criteria.SeeksBackward() {
		q.sql = q.withWhere() + " ORDER BY timestamp ASC, id ASC"
	} else {
		q.sql = q.withWhere() + " ORDER BY timestamp DESC, id DESC"
	}

	return q
}

// paginateWithDetection adds pagination with "has more" detection using LIMIT n+1.
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

// Build returns the final SQL query and arguments.
func (q *DelegationsQueryBuilder) Build() (string, []any) {
	return q.sql, q.args
}

// Helper methods for building SQL

// addWhereCondition records a WHERE condition; conditions are ANDed together when the
// clause is rendered. sqlClause has one %d per value, each replaced by its placeholder.
func (q *DelegationsQueryBuilder) addWhereCondition(sqlClause string, values ...any) {
	placeholders := make([]any, len(values))
	for i, value := range values {
		placeholders[i] = q.nextPlaceholder()
		q.args = append(q.args, value)
	}

	q.where = append(q.where, fmt.Sprintf(sqlClause, placeholders...))
}

// withWhere returns the SQL so far with the WHERE clause rendered.
func (q *DelegationsQueryBuilder) withWhere() string {
	if len(q.where) == 0 {
		return q.sql
	}

	return q.sql + " WHERE " + strings.Join(q.where, " AND ")
}

// addParameter adds a SQL clause with a parameter.
func (q *DelegationsQueryBuilder) addParameter(sqlClause string, value any) {
	placeholder := q.nextPlaceholder()
	q.sql += " " + fmt.Sprintf(sqlClause, placeholder)
	q.args = append(q.args, value)
}

// nextPlaceholder returns the next PostgreSQL placeholder ($1, $2, etc.)
func (q *DelegationsQueryBuilder) nextPlaceholder() int {
	return len(q.args) + 1
}
