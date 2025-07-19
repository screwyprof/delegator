package dbrow

import (
	"time"
)

// Delegation represents a delegation record as queried from the database
type Delegation struct {
	ID        int64     `db:"id"`
	Timestamp time.Time `db:"timestamp"`
	Amount    int64     `db:"amount"`
	Delegator string    `db:"delegator"`
	Level     int64     `db:"level"`
}
