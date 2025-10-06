package util

import "time"

// Timer is a lightweight helper to measure elapsed durations.
type Timer struct {
	start time.Time
}

// StartTimer creates a new timer starting at current time.
func StartTimer() Timer {
	return Timer{start: time.Now()}
}

// ElapsedMs returns the elapsed milliseconds since start.
func (t Timer) ElapsedMs() int64 {
	if t.start.IsZero() {
		return 0
	}
	return time.Since(t.start).Milliseconds()
}
