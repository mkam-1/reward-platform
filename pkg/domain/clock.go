package domain

import "time"

// Clock abstracts time.Now so Engine's behavior is deterministic in tests:
// production code uses SystemClock, tests inject a fake that returns a
// fixed time.
type Clock interface {
	Now() time.Time
}

// SystemClock is the production Clock backed by the real wall clock.
type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now() }
