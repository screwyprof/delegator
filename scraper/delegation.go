package scraper

import "time"

// Delegation represents a delegation in the scraper domain model
// This is the canonical representation used within the scraper service
type Delegation struct {
	ID        int64
	Level     int64
	Timestamp time.Time
	Delegator string
	Amount    int64
}
