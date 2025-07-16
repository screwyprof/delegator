package dbrow

import (
	"time"

	"github.com/screwyprof/delegator/scraper"
)

// Delegation represents a delegation record as stored in the database
type Delegation struct {
	ID        int64     `db:"id"`
	Timestamp time.Time `db:"timestamp"`
	Amount    int64     `db:"amount"`
	Delegator string    `db:"delegator"`
	Level     int64     `db:"level"`
	// created_at is handled by database DEFAULT CURRENT_TIMESTAMP
}

// ScraperDelegationsToRows converts scraper delegations directly to [][]any for pgx.CopyFromRows
func ScraperDelegationsToRows(delegations []scraper.Delegation) [][]any {
	rows := make([][]any, len(delegations))

	for i, d := range delegations {
		rows[i] = []any{
			d.ID,
			d.Timestamp,
			d.Amount,
			d.Delegator,
			d.Level,
		}
	}

	return rows
}
