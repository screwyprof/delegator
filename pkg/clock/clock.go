// Package clock provides time abstractions for production and testing
package clock

import "time"

// SystemClock provides production time implementation using the standard library
type SystemClock struct{}

// After returns a channel that sends the current time after the specified duration
func (SystemClock) After(d time.Duration) <-chan time.Time {
	return time.After(d)
}

// Now returns the current time
func (SystemClock) Now() time.Time {
	return time.Now()
}
