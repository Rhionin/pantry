package server

import (
	"sync"
	"time"
)

// fakeClock is an injectable clock for tests. Background revalidation
// goroutines call Now concurrently with the test body calling Advance, so
// access is mutex-guarded.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

// newFakeClock creates a fakeClock starting at start.
func newFakeClock(start time.Time) *fakeClock {
	return &fakeClock{now: start}
}

// Now returns the clock's current time. Its signature matches the
// func() time.Time fields on product.Refresher.Now and
// product.LookupService.Now, so a *fakeClock's Now method can be assigned
// directly (e.g. refresher.Now = clock.Now).
func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance moves the clock forward by d, letting tests move time instead of
// waiting for it.
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}
